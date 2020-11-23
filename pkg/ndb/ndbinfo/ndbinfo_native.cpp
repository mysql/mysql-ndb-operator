/*
   Copyright (c) 2003, 2018, Oracle and/or its affiliates.

   Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
*/


//#include <ndb_global.h>
//#include <ndb_opts.h>

#include <NdbApi.hpp>
#include "../src/ndbapi/NdbInfo.hpp"
#include <NdbSleep.h>

#include <iostream>

#include <ndbinfo_native.hpp>

typedef struct ndb_result_ctx {

  const NdbInfo::Table * pTab;
  NdbInfoScanOperation * pScan;
  NdbInfo *              pInfo;

  NDB_ROW                row;
  Vector<const NdbInfoRecAttr*> recAttrs;

} ndb_result_ctx;

static int delay = 5;

using namespace std;

extern "C"
NDB * ndb_initialize()
{
  ndb_init();
  NDB * p = new NDB;
  return p;
}

extern "C"
void ndb_free(NDB * ndb)
{
  delete ndb;
}

extern "C"
int ndb_connect(NDB * ndb, char * connectstring)
{
  Ndb_cluster_connection * pcon = new Ndb_cluster_connection(connectstring);
  pcon->set_name("cgo ndbexporter");

  if(pcon->connect(4, 5, 1) != 0)
  {
    cout << "Unable to connect to management server." << endl;
    return 1;
  }

  if (pcon->wait_until_ready(30,0) < 0)
  {
    cout << "Cluster nodes not ready in 30 seconds." << endl;
    return 1;
  }

  ndb->pconnection = pcon;

  return 0;
}

extern "C"
NDB_RESULT * ndb_info_select_all(NDB * ndb, char * tablename)
{

  NdbInfo * pInfo = new NdbInfo((Ndb_cluster_connection *)ndb->pconnection, "");

  if (!pInfo->init())
  {
    cout << "Failed to init ndbinfo!" << endl;
    return nullptr;
  }


  const NdbInfo::Table * pTab = 0;
  int res = pInfo->openTable(tablename, &pTab);
  if (res != 0)
  {
    cout << "Failed to open: " << tablename << ", res: " << res << endl;
    return nullptr;
  }

  unsigned cols = pTab->columns();

  NDB_RESULT * pres = new NDB_RESULT;

  pres->fields = new NDB_FIELD[cols]; 
  pres->field_count = cols;

  for (unsigned i = 0; i < cols; i++)
  {
    const NdbInfo::Column * pCol = pTab->getColumn(i);

    pres->fields[i].name = new char[strlen(pCol->m_name.c_str()) + 1];
    strcpy(pres->fields[i].name, pCol->m_name.c_str());

    if(pCol->m_type == NdbInfo::Column::String) {
      pres->fields[i].coltype = ndb_column_type_string;
    } else {
      pres->fields[i].coltype = ndb_column_type_int64;
    }
  }

  /*
  for (unsigned i = 0; i < cols; i++)
  {
    if(pres->fields[i].coltype == ndb_column_type_string) {
      cout << "[" << i << "]" << pres->fields[i].name << ": " << "string" << endl;
    } else {
      cout << "[" << i << "]" << pres->fields[i].name << ": " << "int" << endl;
    }
  }
  */

  ndb_result_ctx * ctx = new ndb_result_ctx;
  ctx->pTab = pTab;
  ctx->pInfo = pInfo;
  ctx->pScan = NULL;

  pres->pcontext = (void *)ctx;

  pres->tablename = new char[strlen(tablename) + 1];
  strcpy(pres->tablename, tablename);

  return pres;
}

extern "C"
void ndb_free_result(NDB_RESULT * res) {

  ndb_result_ctx * ctx = (ndb_result_ctx *)res->pcontext;
  NdbInfo * pInfo = ctx->pInfo;

  for (unsigned i = 0; i < res->field_count; i++) {
    delete[] res->fields[i].name;
  }
  delete[] res->fields;

  pInfo->releaseScanOperation(ctx->pScan);
  pInfo->closeTable(ctx->pTab);

  delete[] ctx->row;

  delete ctx;

  delete[] res->tablename;
  delete res;
}

int ndb_info_init_scan(NDB_RESULT * res, ndb_result_ctx * ctx) {

    const uint32_t batchsizerows = 32;

    NdbInfoScanOperation * pScan = 0;
    NdbInfo * pInfo = ctx->pInfo;

    int r = ctx->pInfo->createScanOperation(ctx->pTab, &pScan, batchsizerows);
    if (r != 0)
    {
      cout << "Failed to createScan: " << res->tablename << ", result code: " << r << endl;
      pInfo->closeTable(ctx->pTab);
      return 1;
    }
    ctx->pScan = pScan;

    if (pScan->readTuples() != 0)
    {
      cout << "scanOp->readTuples failed" << endl;
      return 1;
    }
    
    for (unsigned i = 0; i < ctx->pTab->columns(); i++)
    {
      const NdbInfoRecAttr* pRec = pScan->getValue(i);
      if (pRec == 0)
      {
        cout << "Failed to getValue(" << i << ")" << endl;
        return 1;
      }
      ctx->recAttrs.push_back(pRec);
    }

    ctx->row = new NDB_COLUMN[ctx->pTab->columns()];

    if(pScan->execute() != 0)
    {
      cout << "scanOp->execute failed" << endl;
      return 1;
    }

    return 0;
}

extern "C"
NDB_ROW ndb_fetch_row(NDB_RESULT * res) {

  ndb_result_ctx * ctx = (ndb_result_ctx *)res->pcontext;

  if(!ctx->pScan) {
    if(ndb_info_init_scan(res, ctx)) {
      return (nullptr);
    }
  }

  if(ctx->pScan->nextResult() == 1)
  {
    for (unsigned i = 0; i < ctx->pTab->columns(); i++)
    {
      if (ctx->recAttrs[i]->isNULL())
      {
        ctx->row[i].isNull = true;
      }
      else
      {

        ctx->row[i].isNull = false;
        switch(ctx->pTab->getColumn(i)->m_type){
        case NdbInfo::Column::String:
          ctx->row[i].pstr = (const char *)ctx->recAttrs[i]->c_str();
          // cout << ctx->row[i].pstr;
          break;
        case NdbInfo::Column::Number:
          ctx->row[i].num = ctx->recAttrs[i]->u_32_value();
          // cout << ctx->row[i].num;
          break;
        case NdbInfo::Column::Number64:
          ctx->row[i].num = ctx->recAttrs[i]->u_64_value();
          // cout << ctx->row[i].num;
          break;
        }
      }
      // cout << ", ";
    }
    // cout << endl;
    return ctx->row;
  }

  return 0;
}

extern "C"
uint16_t ndb_field_count(NDB_RESULT * res) {
  ndb_result_ctx * ctx = (ndb_result_ctx *)res->pcontext;
  return ctx->pTab->columns();
}

extern "C"
NDB_FIELD * ndb_get_field_direct(NDB_RESULT * res, uint16_t fieldnr) {
  return &(res->fields[fieldnr]);
}

template class Vector<const NdbInfoRecAttr*>;
