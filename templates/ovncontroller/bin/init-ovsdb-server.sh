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
TLSOptions="--certificate=/etc/pki/tls/certs/ovndb.crt --private-key=/etc/pki/tls/private/ovndb.key --ca-cert=/etc/pki/tls/certs/ovndbca.crt"
DBOptions="--db ssl:ovsdbserver-nb.openstack.svc.cluster.local:6641"

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
  ## Need to lower chassis priority
  # First get the system-id
  chassis_id=$(ovs-vsctl get Open_Vswitch . external_ids:system-id)
  nb_output=$(ovn-nbctl $DBOptions $TLSOptions --columns=_uuid,priority find Gateway_Chassis chassis_name=$chassis_id)
  row_uuid=$(echo "$nb_output" | grep "_uuid" | cut -d':' -f2 | xargs)
  priority=$(echo "$nb_output" | grep "priority" | cut -d':' -f2 | xargs)
  # Save priority to be able to restore it later (It's overwritting, not appending, hence no check)
  echo $priority > /var/lib/openvswitch/old_priority
  # Set lower priority (lowest value possible 0)
  ovn-nbctl $DBOptions $TLSOptions set Gateway_Chassis $row_uuid priority=0
  sleep 10
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
