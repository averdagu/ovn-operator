#!/bin//bash
#
# Copyright 2022 Red Hat Inc.
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

source $(dirname $0)/functions

# From now on, we should exit immediatelly when any command exits with non-zero status
set -ex

# Rm previous flows just in case
rm /tmp/flows || true

# Store flows
/usr/share/openvswitch/scripts/ovs-save save-flows $bridges > /tmp/flows
ovs-vsctl --no-wait set open_vswitch . other_config:flow-restore-wait=true

# install tcpdump
rpm -U /var/usr/update/ovn26.03-26.03.1-86.el9fdp.x86_64.rpm

/usr/sbin/ovs-vswitchd --pidfile --mlockall --detach --log-file
# Restore flows
eval "$(cat /tmp/flows)"
ovs-vsctl remove open_vswitch . other_config flow-restore-wait
tail -f /var/log/openvswitch/ovs-vswitchd.log
