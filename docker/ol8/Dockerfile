FROM oraclelinux:8-slim

# rpcgen for GCS - as of 8 not part of glibc stuff anymore
# libtirpc-devel for GCS
RUN rpmkeys --import https://repo.mysql.com/RPM-GPG-KEY-mysql && \
microdnf install gcc-toolset-9 && \
microdnf install gcc gcc-c++ && \
microdnf install binutils make && \
microdnf install bison cmake && \
microdnf install openssl openssl-devel && \
microdnf install libtirpc-devel ncurses ncurses-devel && \
microdnf install -y --enablerepo=ol8_codeready_builder rpcgen && \
microdnf clean all

# for rpm build only - didn’t really work for me - need to check options for correct directories
RUN microdnf install rpm-build && \
microdnf install cyrus-sasl-devel cyrus-sasl-scram libaio-devel && \
microdnf install numactl-devel openldap-devel time

RUN microdnf install perl && \
microdnf install perl-Data-Dumper perl-Env perl-File-Temp perl-Getopt-Long perl-JSON perl-Memoize perl-Time-HiRes

COPY boost_1_70_0.tar.bz2 /root/rpmbuild/SOURCES/

ENTRYPOINT ["/bin/bash"]
