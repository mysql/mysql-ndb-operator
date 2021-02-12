#ifndef _NDB_NATIVE_HPP_
#define _NDB_NATIVE_HPP_

#define MAX_COLUMNS 512

#include <stdbool.h> 
#include <stdint.h> 

#if defined(__cplusplus)
extern "C" {
#endif

typedef struct NDB {
    void * pconnection;
} NDB;

enum ndb_column_type {
    ndb_column_type_string = 0,
    ndb_column_type_int64  = 1
};

typedef struct NDB_FIELD {
    enum ndb_column_type coltype;
    char * name;
} NDB_FIELD;

typedef struct NDB_COLUMN {
    bool isNull;
    union {
        const char * pstr;
        uint64_t num;
    };
} NDB_COLUMN;

typedef struct NDB_COLUMN* NDB_ROW;

typedef struct NDB_RESULT {
    char * tablename;
    void * pcontext;
    NDB_FIELD * fields;
    uint16_t field_count;
} NDB_RESULT;

NDB * ndb_initialize();
void ndb_free(NDB * ndb);

int ndb_connect(NDB * ndb, char * connectstring);

NDB_RESULT * ndb_info_select_all(NDB * ndb, char * tablename);
void ndb_free_result(NDB_RESULT * res);

NDB_ROW ndb_fetch_row(NDB_RESULT * result);

uint16_t ndb_field_count(NDB_RESULT * );
NDB_FIELD * ndb_get_field_direct(NDB_RESULT *, uint16_t);

#if defined(__cplusplus)
}
#endif

#endif