package mgm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// NDB MGM Client connection.  See go doc ndb/mgm.Client for methods.
type Client struct {
	connection net.Conn
	state      protocolState
}

func (c *Client) execVerbose(req *request, res *response) error {
	res.verbose = true
	req.verbose = true
	return c.exec(req, res)
}

func (c *Client) exec(req *request, res *response) error {
	err := c.send(req)
	if err == nil {
		err = c.recv(res)
	}
	return err
}

//
// Requests
//

/* Request parameter */
type param struct {
	key string
	val string
}

func (p *param) write(w io.Writer, verbose bool) {
	fmt.Fprintf(w, "%s: %s\n", p.key, p.val)
	if verbose {
		fmt.Printf("%s: %s\n", p.key, p.val)
	}
}

/* Request */
type request struct {
	id      string
	params  []*param
	verbose bool // Copy protocol exchange to stdout
}

func newRequest(id string) *request {
	r := new(request)
	r.id = id
	return r
}

// Add a parameter to the request
func (req *request) bindStr(key string, value string) {
	p := new(param)
	p.key = key
	p.val = value
	req.params = append(req.params, p)
}

func (req *request) bindInt(key string, value int) {
	req.bindStr(key, strconv.FormatInt(int64(value), 10))
}

func (req *request) send(w net.Conn) error {
	fmt.Fprintln(w, req.id)
	if req.verbose {
		println(req.id)
	}
	for _, p := range req.params {
		p.write(w, req.verbose)
	}
	_, err := fmt.Fprintf(w, "\n")
	return err
}

//
// Responses
//

/* Bound response parameter */
type boundParam struct {
	intVar *int
	strVar *string
}

func (p *boundParam) read(content string) error {
	var err error
	if p.intVar != nil {
		_, err = fmt.Sscanf(content, "%d", p.intVar)
	} else if p.strVar != nil {
		*(p.strVar) = content
	}
	return err
}

/* Response */
type response struct {
	Timeout    time.Duration // can be set by user
	ExpectData bool          // expect server to send bulk data
	Data       string        // response bulk data
	id         string        // identifier string in response header
	result     string        // can be bound to the "result:" header
	verbose    bool          // Copy protocol exchange to stdout
	params     map[string]*boundParam
	boundMap   *map[string]string
}

func newResponse(id string) *response {
	r := new(response)
	r.id = id
	r.Timeout = 5 * time.Second
	r.params = make(map[string]*boundParam)
	return r
}

// Bind an integer pointer to a response parameter
func (r *response) bindInt(name string, bindVar *int) {
	p := new(boundParam)
	p.intVar = bindVar
	r.params[name] = p
}

// Bind a string pointer to a response parameter
func (r *response) bindStr(name string, bindVar *string) {
	p := new(boundParam)
	p.strVar = bindVar
	r.params[name] = p
}

// Indicate that a response parameter can be ignored
func (r *response) ignore(name string) {
	p := new(boundParam)
	r.params[name] = p
}

// Bind the "result" header internally
func (r *response) bindResultHeader() {
	p := new(boundParam)
	p.strVar = &r.result
	r.params["result"] = p
	r.result = "(no result header)" // in case it does not arrive
}

// Bind a map to hold all (otherwise unbound) response parameters
func (r *response) bindMap(unmatchingHeaderMap *map[string]string) {
	r.boundMap = unmatchingHeaderMap
}

// bulk data consists of base64-encoded lines terminated by a blank line
func (res *response) readBulkData(scanner *bufio.Scanner) error {
	if res.verbose {
		fmt.Printf("Reading bulk data:\n")
	}
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 { // Finished, having reached \n\n
			break
		}
		res.Data += line + "\n"
		if res.verbose {
			println(line)
		}
	}
	return scanner.Err()
}

func (res *response) readHeaders(scanner *bufio.Scanner) error {
	var err error

	// Get response line
	if scanner.Scan() {
		line := scanner.Text()
		if line != res.id {
			err = fmt.Errorf("unexpected response '%s'", line)
		}
	}

	// If no headers are bound, we are done. There will not be a blank line.
	if len(res.params) == 0 {
		return err
	}

	// Get response headers
	for err == nil && scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 { // Finished, having reached \n\n
			return nil
		}
		if res.verbose {
			println(line)
		}
		// Sometimes there's a space, sometimes there isn't...
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) < 2 {
			parts = strings.SplitN(line, ":", 2)
		}
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]
			p := res.params[key]
			if p != nil { // bound response header
				err = p.read(value)
			} else if res.boundMap != nil {
				(*res.boundMap)[key] = value
			} else { // unexpected header
				err = fmt.Errorf("unexpected response header '%s'", key)
			}
		} else {
			err = fmt.Errorf("unexpected response line '%s'", line)
		}
	}

	return err
}

type protocolState byte

const ( // protocolState
	not_connected protocolState = iota
	connected
	request_sent    // client has sent request and awaits response
	reading_headers // client is reading response headers
	bulk_data       // client is reading bulk data
)

func (s protocolState) String() string {
	switch s {
	case not_connected:
		return "not_connected"
	case connected:
		return "connected"
	case request_sent:
		return "request_sent"
	case reading_headers:
		return "reading_headers"
	case bulk_data:
		return "bulk_data"
	}
	return "unexpected"
}

func (c *Client) requireState(expected protocolState) error {
	if c.state == expected {
		return nil
	}
	return fmt.Errorf("bad connection state: %s", c.state)
}

func (c *Client) setStateIfOk(next protocolState, err error) error {
	if err == nil {
		c.state = next
	}
	return err
}

// Connect to an NDB management server. Address is passed to Dial().
func (c *Client) Connect(address string) error {
	err := c.requireState(not_connected)

	if err != nil {
		return err
	}
	if c.connection != nil {
		return errors.New("already connected")
	}
	c.connection, err = net.Dial("tcp", address)
	return c.setStateIfOk(connected, err)
}

// Disconnect from management server
func (c *Client) Disconnect() error {
	var err error
	if c.connection == nil {
		err = errors.New("not connected")
	} else {
		c.connection.Close()
		c.connection = nil
	}
	c.state = not_connected
	return err
}

func (c *Client) send(req *request) error {
	err := c.requireState(connected)

	if err == nil {
		err = req.send(c.connection)
	}
	return c.setStateIfOk(request_sent, err)
}

func (c *Client) recv(res *response) error {
	err := c.requireState(request_sent)

	if err != nil {
		return err
	}

	// Set the read timeout; defer error catching until read()
	_ = c.connection.SetReadDeadline(time.Now().Add(res.Timeout))

	// Read the headers
	c.state = reading_headers

	scanner := bufio.NewScanner(c.connection)
	scanner.Split(bufio.ScanLines)

	err = res.readHeaders(scanner)

	// Check an internally-bound result header
	if res.result != "" {
		if !(res.result == "Ok" || res.result == "ok" || res.result == "0") {
			err = fmt.Errorf("Result: %s", res.result)
		}
	}

	// Read bulk data. If bulk data is not expected, the scanner should be empty.
	if err == nil {
		if res.ExpectData {
			c.state = bulk_data
			err = res.readBulkData(scanner)
		} else {
			c.connection.SetReadDeadline(time.Now())
			if scanner.Scan() {
				err = fmt.Errorf("Unexpected response data after headers")
			}
		}
	}

	// On success, connection state cycles back to connected
	return c.setStateIfOk(connected, err)
}
