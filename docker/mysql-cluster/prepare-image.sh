#!/bin/bash
# Copyright (c) 2021, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/


# Create directories needed by mysqld and make them writable by group 0
mysql_dirs="/var/lib/mysql /var/lib/mysql-files /var/lib/mysql-keyring /var/run/mysqld"

for dir in $mysql_dirs; do
    mkdir -p $dir
    chmod g+rwx $dir
    chgrp -R 0 $dir
done
