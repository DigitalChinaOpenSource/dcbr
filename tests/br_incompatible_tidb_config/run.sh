#!/bin/bash
#
# Copyright 2020 PingCAP, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# See the License for the specific language governing permissions and
# limitations under the License.

set -eu

cur=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
source $cur/../_utils/run_services

DB="$TEST_NAME"

# prepare database
echo "Restart cluster with alter-primary-key = true, max-index-length=12288"
start_services "$cur"

run_sql "drop schema if exists $DB;"
run_sql "create schema $DB;"

# test alter pk issue https://github.com/DigitalChinaOpenSource/dcbr/issues/215
TABLE="t1"
INCREMENTAL_TABLE="t1inc"

run_sql "create table $DB.$TABLE (a int primary key, b int unique);"
run_sql "insert into $DB.$TABLE values (42, 42);"

# backup
run_br --pd $PD_ADDR backup db --db "$DB" -s "local://$TEST_DIR/$DB$TABLE"

run_sql "create table $DB.$INCREMENTAL_TABLE (a int primary key, b int unique);"
run_sql "insert into $DB.$INCREMENTAL_TABLE values (42, 42);"

# drop pk
run_sql "alter table $DB.$INCREMENTAL_TABLE drop primary key"
run_sql "drop table $DB.$INCREMENTAL_TABLE"
run_sql "create table $DB.$INCREMENTAL_TABLE like $DB.$TABLE"
run_sql "insert into $DB.$INCREMENTAL_TABLE values (42, 42);"

# incremental backup
run_br --pd $PD_ADDR backup db --db "$DB" -s "local://$TEST_DIR/$DB$INCREMENTAL_TABLE"

# restore
run_sql "drop schema $DB;"

run_br --pd $PD_ADDR restore db --db "$DB" -s "local://$TEST_DIR/$DB$TABLE"

run_br --pd $PD_ADDR restore db --db "$DB" -s "local://$TEST_DIR/$DB$INCREMENTAL_TABLE"

run_sql "drop schema $DB;"
run_sql "create schema $DB;"

# test max-index-length issue https://github.com/DigitalChinaOpenSource/dcbr/issues/217
TABLE="t2"
run_sql "create table $DB.$TABLE (a varchar(3072) primary key);"
run_sql "insert into $DB.$TABLE values ('42');"

# backup
run_br --pd $PD_ADDR backup db --db "$DB" -s "local://$TEST_DIR/$DB$TABLE"

# restore
run_sql "drop schema $DB;"
run_br --pd $PD_ADDR restore db --db "$DB" -s "local://$TEST_DIR/$DB$TABLE"

run_sql "drop schema $DB;"

# we need set auto_random to true and remove alter-primary-key otherwise we will get error
# invalid config allow-auto-random is unavailable when alter-primary-key is enabled

# enable column attribute `auto_random` to be defined on the primary key column.
cat > $cur/config/tidb.toml << EOF
[experimental]
allow-auto-random = true
EOF

echo "Restart cluster with allow-auto-random=true"
start_services "$cur"

# test auto random issue https://github.com/DigitalChinaOpenSource/dcbr/issues/228
TABLE="t3"
INCREMENTAL_TABLE="t3inc"
run_sql "create schema $DB;"
run_sql "create table $DB.$TABLE (a bigint(11) NOT NULL /*T!30100 AUTO_RANDOM(5) */, PRIMARY KEY (a))"
run_sql "insert into $DB.$TABLE values ('42');"

# Full backup
run_br --pd $PD_ADDR backup db --db "$DB" -s "local://$TEST_DIR/$DB$TABLE"

run_sql "create table $DB.$INCREMENTAL_TABLE (a bigint(11) NOT NULL /*T!30100 AUTO_RANDOM(5) */, PRIMARY KEY (a))"
run_sql "insert into $DB.$INCREMENTAL_TABLE values ('42');"

# incremental backup test for execute DDL
last_backup_ts=$(br validate decode --field="end-version" -s "local://$TEST_DIR/$DB$TABLE" | tail -n1)
run_br --pd $PD_ADDR backup db --db "$DB" -s "local://$TEST_DIR/$DB$INCREMENTAL_TABLE" --lastbackupts $last_backup_ts

run_sql "drop schema $DB;"

# full restore
run_br --pd $PD_ADDR restore db --db "$DB" -s "local://$TEST_DIR/$DB$TABLE"
# incremental restore
run_br --pd $PD_ADDR restore db --db "$DB" -s "local://$TEST_DIR/$DB$INCREMENTAL_TABLE"

run_sql "drop schema $DB;"

# test auto random issue https://github.com/DigitalChinaOpenSource/dcbr/issues/241
TABLE="t4"
run_sql "create schema $DB;"
run_sql "create table $DB.$TABLE(a bigint key auto_random(5));"
run_sql "insert into $DB.$TABLE values (),(),(),(),();"

# Table backup
run_br --pd $PD_ADDR backup table --db "$DB" --table "$TABLE" -s "local://$TEST_DIR/$DB$TABLE"
run_sql "drop schema $DB;"

# Table restore, restore normally without Duplicate entry
run_br --pd $PD_ADDR restore table --db "$DB" --table "$TABLE" -s "local://$TEST_DIR/$DB$TABLE"

# run insert after restore
run_sql "insert into $DB.$TABLE values (),(),(),(),();"

row_count=$(run_sql "select a & b'0000011111111111111111111111111' from $DB.$TABLE;" | grep -v "a" | grep -v "-" | sort -u | wc -l)
if [ "$row_count" -ne "10" ];then
    echo "TEST: [$TEST_NAME] failed!, because auto_random didn't rebase"
    exit 1
fi

echo "Restart service with normal"
start_services
