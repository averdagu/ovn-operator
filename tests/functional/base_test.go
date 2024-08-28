/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

*/

package functional_test

import (
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/gomega" //revive:disable:dot-imports
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	infranetworkv1 "github.com/openstack-k8s-operators/infra-operator/apis/network/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
)

const (
	timeout  = time.Second * 10
	interval = timeout / 100
)

func GetDefaultOVNNorthdSpec() ovnv1.OVNNorthdSpec {
	return ovnv1.OVNNorthdSpec{
		OVNNorthdSpecCore: ovnv1.OVNNorthdSpecCore{},
	}
}

func GetTLSOVNNorthdSpec() ovnv1.OVNNorthdSpec {
	spec := GetDefaultOVNNorthdSpec()
	spec.TLS = tls.SimpleService{
		Ca: tls.Ca{
			CaBundleSecretName: CABundleSecretName,
		},
		GenericService: tls.GenericService{
			SecretName: ptr.To(OvnDbCertSecretName),
		},
	}
	return spec
}

func GetOVNNorthd(name types.NamespacedName) *ovnv1.OVNNorthd {
	return ovn.GetOVNNorthd(name)
}

func OVNNorthdConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := ovn.GetOVNNorthd(name)
	return instance.Status.Conditions
}

func GetDefaultOVNDBClusterSpec() ovnv1.OVNDBClusterSpec {
	return ovnv1.OVNDBClusterSpec{
		OVNDBClusterSpecCore: ovnv1.OVNDBClusterSpecCore{
			DBType:         ovnv1.NBDBType,
			StorageRequest: "1G",
			StorageClass:   "local-storage",
		},
	}
}

func GetTLSOVNDBClusterSpec() ovnv1.OVNDBClusterSpec {
	spec := GetDefaultOVNDBClusterSpec()
	spec.TLS = tls.SimpleService{
		Ca: tls.Ca{
			CaBundleSecretName: CABundleSecretName,
		},
		GenericService: tls.GenericService{
			SecretName: ptr.To(OvnDbCertSecretName),
		},
	}
	return spec
}

func CreateOVNDBCluster(namespace string, spec ovnv1.OVNDBClusterSpec) client.Object {
	name := ovn.CreateOVNDBCluster(namespace, spec)
	return ovn.GetOVNDBCluster(name)
}

func OVNDBClusterConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := ovn.GetOVNDBCluster(name)
	return instance.Status.Conditions
}

func ScaleDBCluster(name types.NamespacedName, replicas int32) {
	Eventually(func(g Gomega) {
		c := ovn.GetOVNDBCluster(name)
		*c.Spec.Replicas = replicas
		g.Expect(k8sClient.Update(ctx, c)).Should(Succeed())
	}).Should(Succeed())
}

// CreateOVNDBClusters Creates NB and SB OVNDBClusters
func CreateOVNDBClusters(namespace string, nad map[string][]string, replicas int32) []types.NamespacedName {
	dbs := []types.NamespacedName{}
	for _, db := range []string{ovnv1.NBDBType, ovnv1.SBDBType} {
		spec := GetDefaultOVNDBClusterSpec()
		stringNad := ""
		// OVNDBCluster doesn't allow multiple NADs, hence map len
		// must be <= 1
		Expect(len(nad)).Should(BeNumerically("<=", 1))
		for k := range nad {
			if strings.Contains(k, "/") {
				// k = namespace/nad_name, split[1] will return nad_name (e.g. internalapi)
				stringNad = strings.Split(k, "/")[1]
			}
		}
		if len(nad) != 0 {
			// nad format needs to be map[string][]string{namespace + "/" + nad_name: ...} or empty
			Expect(stringNad).ToNot(Equal(""))
		}
		spec.DBType = db
		spec.NetworkAttachment = stringNad
		spec.Replicas = &replicas

		instance := CreateOVNDBCluster(namespace, spec)
		instanceName := types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}

		dbName := "nb"
		if db == ovnv1.SBDBType {
			dbName = "sb"
		}
		statefulSetName := types.NamespacedName{
			Namespace: instance.GetNamespace(),
			Name:      "ovsdbserver-" + dbName,
		}
		th.SimulateStatefulSetReplicaReadyWithPods(
			statefulSetName,
			nad,
		)
		// Ensure that PODs are ready and DBCluster have been reconciled
		// with all information (Status.DBAddress and internalDBAddress
		// are set at the end of the reconcileService)
		Eventually(func(g Gomega) {
			ovndbcluster := ovn.GetOVNDBCluster(instanceName)
			endpoint := ""
			// Check External endpoint when NAD is set
			if len(nad) == 0 {
				endpoint, _ = ovndbcluster.GetInternalEndpoint()
			} else {
				endpoint, _ = ovndbcluster.GetExternalEndpoint()
			}
			g.Expect(endpoint).ToNot(BeEmpty())
		}).Should(Succeed())

		dbs = append(dbs, instanceName)

	}

	logger.Info("OVNDBClusters created", "OVNDBCluster", dbs)
	return dbs
}

// DeleteOVNDBClusters Delete OVN DBClusters
func DeleteOVNDBClusters(names []types.NamespacedName) {
	for _, db := range names {
		th.DeleteInstance(ovn.GetOVNDBCluster(db))
	}
}

// GetOVNDBCluster Get OVNDBCluster
func GetOVNDBCluster(name types.NamespacedName) *ovnv1.OVNDBCluster {
	return ovn.GetOVNDBCluster(name)
}

// GetConfigMap -
func GetConfigMap(name types.NamespacedName) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, cm)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return cm
}

// GetDaemonSet -
func GetDaemonSet(name types.NamespacedName) *appsv1.DaemonSet {
	ds := &appsv1.DaemonSet{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, ds)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return ds
}

// ListDaemonsets -
func ListDaemonsets(namespace string) *appsv1.DaemonSetList {
	dss := &appsv1.DaemonSetList{}
	Expect(k8sClient.List(ctx, dss, client.InNamespace(namespace))).Should(Succeed())
	return dss
}

// SimulateDaemonsetNumberReady -
func SimulateDaemonsetNumberReady(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		ds := GetDaemonSet(name)
		ds.Status.NumberReady = 1
		ds.Status.DesiredNumberScheduled = 1
		g.Expect(k8sClient.Status().Update(ctx, ds)).To(Succeed())

	}, timeout, interval).Should(Succeed())
	logger.Info("Simulated daemonset success", "on", name)
}

func GetDefaultOVNControllerSpec() ovnv1.OVNControllerSpec {
	return ovnv1.OVNControllerSpec{}
}

func GetTLSOVNControllerSpec() ovnv1.OVNControllerSpec {
	spec := GetDefaultOVNControllerSpec()
	spec.TLS = tls.SimpleService{
		Ca: tls.Ca{
			CaBundleSecretName: CABundleSecretName,
		},
		GenericService: tls.GenericService{
			SecretName: ptr.To(OvnDbCertSecretName),
		},
	}
	return spec
}

func CreateOVNController(namespace string, spec ovnv1.OVNControllerSpec) client.Object {

	name := ovn.CreateOVNController(namespace, spec)
	return ovn.GetOVNController(name)
}

func GetOVNController(name types.NamespacedName) *ovnv1.OVNController {
	return ovn.GetOVNController(name)
}

func OVNControllerConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := ovn.GetOVNController(name)
	return instance.Status.Conditions
}

func SimulateDaemonsetNumberReadyWithPods(name types.NamespacedName, networkIPs map[string][]string) {
	ds := GetDaemonSet(name)

	for i := 0; i < int(1); i++ {
		pod := &corev1.Pod{
			ObjectMeta: ds.Spec.Template.ObjectMeta,
			Spec:       ds.Spec.Template.Spec,
		}
		pod.ObjectMeta.Namespace = name.Namespace
		pod.ObjectMeta.Name = name.Name
		pod.ObjectMeta.Labels = map[string]string{
			"service": name.Name,
		}

		// NodeName required for getOVNControllerPods
		pod.Spec.NodeName = name.Name

		// If networkIPs is empty, skip adding network status annotations
		if len(networkIPs) > 0 {
			var netStatus []networkv1.NetworkStatus
			for network, IPs := range networkIPs {
				netStatus = append(
					netStatus,
					networkv1.NetworkStatus{
						Name: network,
						IPs:  IPs,
					},
				)
			}
			netStatusAnnotation, err := json.Marshal(netStatus)
			Expect(err).NotTo(HaveOccurred())
			pod.Annotations[networkv1.NetworkStatusAnnot] = string(netStatusAnnotation)
		}

		Expect(k8sClient.Create(ctx, pod)).Should(Succeed())
	}

	Eventually(func(g Gomega) {
		ds := GetDaemonSet(name)
		ds.Status.NumberReady = 1
		ds.Status.DesiredNumberScheduled = 1
		g.Expect(k8sClient.Status().Update(ctx, ds)).To(Succeed())

	}, timeout, interval).Should(Succeed())

	logger.Info("Simulated daemonset success", "on", name)
}

func CreateNAD(name types.NamespacedName) *networkv1.NetworkAttachmentDefinition {
	nad := &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
		},
		Spec: networkv1.NetworkAttachmentDefinitionSpec{
			Config: "",
		},
	}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Create(ctx, nad)).Should(Succeed())
	}).Should(Succeed())

	return nad
}

func GetNAD(name types.NamespacedName) *networkv1.NetworkAttachmentDefinition {
	nad := &networkv1.NetworkAttachmentDefinition{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, nad)).Should(Succeed())
	}).Should(Succeed())

	return nad
}

func GetDNSData(name types.NamespacedName) *infranetworkv1.DNSData {
	dns := &infranetworkv1.DNSData{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, dns)).Should(Succeed())
	}).Should(Succeed())

	return dns
}

func GetDNSDataHostsList(namespace string, dnsEntryName string) []infranetworkv1.DNSHost {
	dnsEntry := GetDNSData(types.NamespacedName{Name: dnsEntryName, Namespace: namespace})

	return dnsEntry.Spec.Hosts
}

func CheckDNSDataContainsIP(namespace string, dnsEntryName string, ip string) bool {
	hostList := GetDNSDataHostsList(namespace, dnsEntryName)
	for _, host := range hostList {
		if host.IP == ip {
			return true
		}
	}
	return false
}

func GetPod(name types.NamespacedName) *corev1.Pod {
	pod := &corev1.Pod{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, pod)).Should(Succeed())
	}).Should(Succeed())

	return pod
}

func UpdatePod(pod *corev1.Pod) {
	Expect(k8sClient.Update(ctx, pod)).Should(Succeed())
}

func GetServicesListWithLabel(namespace string, labelSelectorMap ...map[string]string) *corev1.ServiceList {
	serviceList := &corev1.ServiceList{}
	serviceListOpts := client.ListOptions{
		Namespace: namespace,
	}
	if len(labelSelectorMap) > 0 {
		for i := 0; i < len(labelSelectorMap); i++ {
			for key, value := range labelSelectorMap[i] {
				ml := client.MatchingLabels{
					key: value,
				}
				ml.ApplyToList(&serviceListOpts)
			}
		}
	}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.List(ctx, serviceList, &serviceListOpts)).Should(Succeed())
	}).Should(Succeed())

	return serviceList
}
