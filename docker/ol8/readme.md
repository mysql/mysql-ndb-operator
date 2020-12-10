This is a docker image used to compile MySQL for target platform Oracle Linux 8.

It contains everything needed to produce a simple build. 

$ docker run -ti  -v /Users/bo/prg:/Users/bo/prg ol8dev:1.0.0 

$ cmake ../../mysql -DWITH_NDBCLUSTER=1 -DDOWNLOAD_BOOST=1 -DWITH_BOOST=/Users/bo/prg/boost -DWITH_NDB_JAVA=0 -DWITH_NDBAPI_EXAMPLES=0 -DWITH_NDBCLUSTER=1 -DWITH_BUNDLED_MEMCACHED=0 -DWITH_BLACKHOLE_STORAGE_ENGINE=0 -DCMAKE_BUILD_TYPE=Relwithdebinfo

