// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndb

import (
	/*
			  #cgo CFLAGS: -I ./
			  #cgo LDFLAGS: -L./ -lndbinfo_native
			  #include <stdlib.h>
			  #include <stdio.h>
			  #include <ndbinfo_native.hpp>

		  	  int is_null(void* p) {
			    if (NULL == p) {
				  return 1;
			    }
			    return 0;
			  }

			  const char * str_at(NDB_ROW r, int i) {
				return r[i].pstr;
			  }

			  uint64_t int_at(NDB_ROW r, int i) {
				return r[i].num;
			  }

			  void printfield(NDB_FIELD * field) {
				  printf("c : [%d] %d\n", 0, field->coltype);
				  printf("c : [%d] %s\n", 0, field->name);
			  }
	*/
	"C"
)
import (
	"fmt"
	"reflect"
	"unsafe"
)

//type NDB C.struct_NDB

type NdbInfo struct {
	dsn string
	ndb *C.struct_NDB
}

type Rows struct {
	result *C.struct_NDB_RESULT
	row    C.NDB_ROW
}

func NewNdbConnection(dsn string) *NdbInfo {

	cs := C.CString(dsn)
	defer C.free(unsafe.Pointer(cs))

	ndbInfo := &NdbInfo{
		dsn: dsn,
	}

	ndb := C.ndb_initialize()
	C.ndb_connect(ndb, cs)

	ndbInfo.ndb = ndb

	return ndbInfo
}

func (ni *NdbInfo) Free() {
	C.ndb_free(ni.ndb)
}

func (ni *NdbInfo) SelectAll(tablename string) *Rows {

	tablename_c := C.CString(tablename)
	defer C.free(unsafe.Pointer(tablename_c))

	result := &Rows{
		result: C.ndb_info_select_all(ni.ndb, tablename_c),
	}

	return result
}

func (rs *Rows) Next() bool {
	row := C.ndb_fetch_row(rs.result)

	if C.is_null(unsafe.Pointer(row)) == 1 {
		return false
	}

	rs.row = row

	return true
}

func (rs *Rows) NoOfColumns() int {
	return int(C.ndb_field_count(rs.result))
}

func (rs *Rows) IsInt(idx int) bool {

	if idx >= rs.NoOfColumns() {
		return false
	}

	field := C.ndb_get_field_direct(rs.result, C.uint16_t(idx))

	if field.coltype == C.ndb_column_type_int64 {
		return true
	}

	return false
}

func (rs *Rows) IsString(idx int) bool {

	if idx >= rs.NoOfColumns() {
		return false
	}

	field := C.ndb_get_field_direct(rs.result, C.uint16_t(idx))

	if field.coltype == C.ndb_column_type_string {
		return true
	}

	return false
}

func (rs *Rows) AsInt(idx int) int {

	// checking that we don't get end of table
	if C.is_null(unsafe.Pointer(rs.row)) == 1 {
		return 0
	}
	if idx >= int(rs.NoOfColumns()) {
		return 0
	}

	if rs.IsInt(idx) {
		return int(C.int_at(rs.row, C.int(idx)))
	}

	return 0
}

func (rs *Rows) Scan(dst ...interface{}) {

	for i, d := range dst {

		if i > rs.NoOfColumns() {
			return
		}

		vod := reflect.ValueOf(d)
		id := reflect.Indirect(vod)
		switch id.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if rs.IsInt(i) {
				x := int64(C.int_at(rs.row, C.int(i)))
				id.SetInt(x)
			}
			break
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			id.SetUint(8)
			break
		case reflect.String:
			if rs.IsString(i) {
				s := C.GoString(C.str_at(rs.row, C.int(i)))
				id.SetString(s)
			}
			break
		}
	}
}

func (rs *Rows) Close() {
	C.ndb_free_result(rs.result)
}

func test_ndbinfo() {

	//type NDB_RESULT
	//type NDB_FIELD C.struct_NDB_FIELD

	ndbInfo := NewNdbConnection("localhost:14000")

	rows := ndbInfo.SelectAll("cpustat")

	for rows.Next() {
		var (
			node_id   int
			thread_id int
			whatever  int
		)
		rows.Scan(&node_id, &thread_id, &whatever)
		fmt.Println(node_id, thread_id, whatever)

	}
	rows.Close()

	rows = ndbInfo.SelectAll("processes")

	for rows.Next() {
		var (
			node_id      int
			node_type    string
			node_version string
		)
		rows.Scan(&node_id, &node_type, &node_version)
		fmt.Println(node_id, node_type, node_version)

	}
	rows.Close()

	rows = ndbInfo.SelectAll("resources")
	defer rows.Close()

	for rows.Next() {
		var (
			node_id     int
			resource_id int
			reserved    int
			used        int
			max         int
		)
		rows.Scan(&node_id, &resource_id, &reserved, &used, &max)
		fmt.Println(node_id, resource_id, reserved, used, max)

	}

	ndbInfo.Free()
}
