#!/bin/sh
#
# Copyright 2023 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.

# Configs are obtained from ENV variables
isUpdate=${isUpdate:-"false"}

set -ex
source $(dirname $0)/functions
trap wait_for_db_creation EXIT

sleep 3

# If db file is empty, remove it; otherwise service won't start.
# See https://issues.redhat.com/browse/FDP-689 for more details.
if ! [ -s ${DB_FILE} ]; then
    rm -f ${DB_FILE}
fi

# Check if file is created, if not means it's first execution
if [ -f /var/lib/openvswitch/already_executed ]; then
  # File is created, no need to run ovs-ctl
  # Change state to "UPDATE"
  echo "UPDATE" > /var/lib/openvswitch/already_executed
  exit 0
fi

# Initialize or upgrade database if needed
CTL_ARGS="--system-id=random --no-ovs-vswitchd"
/usr/share/openvswitch/scripts/ovs-ctl start $CTL_ARGS
/usr/share/openvswitch/scripts/ovs-ctl stop $CTL_ARGS

if [ ! -f /var/lib/openvswitch/already_executed ]; then
  # If file was not present, set status INIT
  echo "INIT" > /var/lib/openvswitch/already_executed
fi

wait_for_db_creation
trap - EXIT
