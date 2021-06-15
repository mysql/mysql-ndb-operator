cmake_minimum_required (VERSION 2.6)
project (ndbinfo_native)

set (ndbinfo_native_VERSION_MAJOR 0)
set (ndbinfo_native_VERSION_MINOR 1)

set (CMAKE_CXX_STANDARD 14)

if (${CMAKE_SYSTEM_NAME} MATCHES "Darwin")
    set(CMAKE_MACOSX_RPATH 1)
endif()

set (NDB_LIBS ndbclient)
link_directories("${NDB_BUILD_DIR}/lib")

# -DNDB_SOURCE_DIR=/home/bo/prg/mysql -DNDB_BUILD_DIR=/home/bo/prg/mysql-bld/trunk
include_directories(
    "./"
    "${NDB_BUILD_DIR}/storage/ndb/include"
    "${NDB_SOURCE_DIR}/storage/ndb/include/ndbapi"
    "${NDB_SOURCE_DIR}/include"
    "${NDB_SOURCE_DIR}/storage/ndb/include"
    "${NDB_SOURCE_DIR}/storage/ndb/include/portlib"
    "${NDB_SOURCE_DIR}/storage/ndb/src/ndbapi"
    "${NDB_BUILD_DIR}/include"
    )

set(CMAKE_CXX_FLAGS_DEBUG "${CMAKE_CXX_FLAGS_DEBUG} -Wall")
set(CMAKE_CXX_FLAGS_RELEASE "${CMAKE_CXX_FLAGS_RELEASE} -Wall")

set(CMAKE_LIBRARY_OUTPUT_DIRECTORY ./)

add_library(ndbinfo_native SHARED ndbinfo_native)
target_link_libraries (ndbinfo_native ${NDB_LIBS} pthread)
