package main

import (
	"bufio"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ocklin/ndb-operator/pkg/controllers/agent"
	clientset "github.com/ocklin/ndb-operator/pkg/generated/clientset/versioned"
	informers "github.com/ocklin/ndb-operator/pkg/generated/informers/externalversions"
	"github.com/ocklin/ndb-operator/pkg/signals"
	"github.com/ocklin/ndb-operator/pkg/version"

	kubeinformers "k8s.io/client-go/informers"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

var (
	masterURL  string
	kubeconfig string
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}

func handler(w http.ResponseWriter, r *http.Request) {
	name, _ := os.Hostname()
	fmt.Fprintf(w, "Hi there from %s!\n", name)
}

func tcp_client() {

	// connect to server
	conn, _ := net.Dial("tcp", "127.0.0.1:1186")
	err := conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		log.Println("SetReadDeadline failed:", err)
		// do something else, for example create new conn
		return
	}

	defer conn.Close()

	{
		// send to server
		//		fmt.Fprintln(conn, "get status")
		//		fmt.Fprintln(conn, "")

		fmt.Fprintf(conn, "get config_v2\n")
		fmt.Fprintf(conn, "%s: %d\n", "version", 524311)
		fmt.Fprintf(conn, "%s: %d\n", "nodetype", -1)
		fmt.Fprintf(conn, "%s: %d\n", "node", 1)
		fmt.Fprintf(conn, "\n")

		// wait for reply
		var tmp []byte
		var buf []byte
		tmp = make([]byte, 64)
		for {
			n, err := conn.Read(tmp)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Println("read timeout:", err)
					// time out
				} else {
					if err != io.EOF {
						fmt.Println("read error:", err)
					}
				}
				break
			}

			buf = append(buf, tmp[:n]...)
			if n < 64 {
				break
			}
		}

		reply := []string{
			"",
			"get config reply",
			"result",
			"Content-Length",
			"Content-Type",
			"Content-Transfer-Encoding",
		}

		lineno := 1
		scanner := bufio.NewScanner(strings.NewReader(string(buf)))
		base64str := ""
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())

			if lineno < len(reply) {

			} else {
				base64str += line
			}
			fmt.Printf("[%d] %s\n", lineno, line)
			lineno++
		}

		data, err := base64.StdEncoding.DecodeString(base64str)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Printf("%q\n", data)

	}
}

func agent_main() {

	flag.Parse()

	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}
	/* just testing for readiness probes */
	go func() {
		http.HandleFunc("/", handler)
		klog.Fatal(http.ListenAndServe(":8080", nil))
	}()

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	ndbClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building ndb clientset: %s", err.Error())
	}

	k8If := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	ndbOpIf := informers.NewSharedInformerFactory(ndbClient, time.Second*30)

	agentVersion := version.GetBuildVersion()
	klog.Infof("Starting agent version \"%s\"", agentVersion)

	ag, err := agent.New(kubeClient, ndbClient,
		k8If.Core().V1().Pods(),
		ndbOpIf.Ndbcontroller().V1alpha1().Ndbs())

	k8If.Start(stopCh)
	ndbOpIf.Start(stopCh)

	if err = ag.Run(stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}

func main() {
	tcp_client()
}
