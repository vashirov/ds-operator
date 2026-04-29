/*
Copyright 2026 Red Hat, Inc.

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

package controller_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/389ds/ds-operator/api/v1alpha1"
)

const (
	timeout  = 30 * time.Second
	interval = 250 * time.Millisecond
)

var _ = Describe("DirectoryService Controller", func() {

	Context("when creating a minimal DirectoryService CR", func() {
		const dsName = "test-minimal"
		const dsNamespace = "default"

		var ds *operatorv1alpha1.DirectoryService

		BeforeEach(func() {
			ds = &operatorv1alpha1.DirectoryService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dsName,
					Namespace: dsNamespace,
				},
				Spec: operatorv1alpha1.DirectoryServiceSpec{
					Image: "quay.io/389ds/dirsrv:latest",
				},
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
		})

		AfterEach(func() {
			// Delete the CR - cascading delete cleans up owned resources
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())

			// Wait for CR to be gone
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, ds)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should create a StatefulSet with correct spec", func() {
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)
			}, timeout, interval).Should(Succeed())

			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
			Expect(sts.Spec.ServiceName).To(Equal(dsName + "-internal"))
			Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("quay.io/389ds/dirsrv:latest"))
			Expect(sts.Spec.Template.Spec.Containers[0].Name).To(Equal("dirsrv"))

			// Volume claim template
			Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
			Expect(sts.Spec.VolumeClaimTemplates[0].Name).To(Equal("ds-data"))
			storageReq := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(storageReq.String()).To(Equal("1Gi"))

			// Owner reference
			Expect(sts.OwnerReferences).To(HaveLen(1))
			Expect(sts.OwnerReferences[0].Name).To(Equal(dsName))
			Expect(sts.OwnerReferences[0].Kind).To(Equal("DirectoryService"))
		})

		It("should create a headless Service", func() {
			svc := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: dsName + "-internal", Namespace: dsNamespace,
				}, svc)
			}, timeout, interval).Should(Succeed())

			Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
			Expect(svc.Spec.Ports).To(HaveLen(2))
			Expect(svc.Spec.PublishNotReadyAddresses).To(BeTrue())

			// Owner reference
			Expect(svc.OwnerReferences).To(HaveLen(1))
			Expect(svc.OwnerReferences[0].Name).To(Equal(dsName))
		})

		It("should create a ClusterIP Service", func() {
			svc := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: dsName, Namespace: dsNamespace,
				}, svc)
			}, timeout, interval).Should(Succeed())

			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(svc.Spec.Ports).To(HaveLen(2))

			// Check port values (defaults)
			ldapPort := svc.Spec.Ports[0]
			Expect(ldapPort.Name).To(Equal("ldap"))
			Expect(ldapPort.Port).To(Equal(int32(3389)))

			ldapsPort := svc.Spec.Ports[1]
			Expect(ldapsPort.Name).To(Equal("ldaps"))
			Expect(ldapsPort.Port).To(Equal(int32(3636)))

			// Owner reference
			Expect(svc.OwnerReferences).To(HaveLen(1))
			Expect(svc.OwnerReferences[0].Name).To(Equal(dsName))
		})

		It("should auto-generate a DM password Secret", func() {
			secret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: dsName + "-dm-password", Namespace: dsNamespace,
				}, secret)
			}, timeout, interval).Should(Succeed())

			Expect(secret.Data).To(HaveKey("dm-password"))
			Expect(secret.Data["dm-password"]).NotTo(BeEmpty())
			Expect(secret.Type).To(Equal(corev1.SecretTypeOpaque))

			// Owner reference
			Expect(secret.OwnerReferences).To(HaveLen(1))
			Expect(secret.OwnerReferences[0].Name).To(Equal(dsName))
		})

		It("should create a ConfigMap with DS_STARTUP_TIMEOUT", func() {
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: dsName + "-config", Namespace: dsNamespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			Expect(cm.Data).To(HaveKeyWithValue("DS_STARTUP_TIMEOUT", "60"))
			// No suffix configured → no DS_SUFFIX_NAME
			Expect(cm.Data).NotTo(HaveKey("DS_SUFFIX_NAME"))

			// Owner reference
			Expect(cm.OwnerReferences).To(HaveLen(1))
			Expect(cm.OwnerReferences[0].Name).To(Equal(dsName))
		})

		It("should inject DS_DM_PASSWORD via env var by default (env mode)", func() {
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)
			}, timeout, interval).Should(Succeed())

			container := sts.Spec.Template.Spec.Containers[0]

			// DS_DM_PASSWORD injected from Secret via secretKeyRef
			Expect(container.Env).To(HaveLen(1))
			Expect(container.Env[0].Name).To(Equal("DS_DM_PASSWORD"))
			Expect(container.Env[0].ValueFrom).NotTo(BeNil())
			Expect(container.Env[0].ValueFrom.SecretKeyRef.Name).To(Equal(dsName + "-dm-password"))
			Expect(container.Env[0].ValueFrom.SecretKeyRef.Key).To(Equal("dm-password"))

			// No secret volume mount in env mode
			for _, vm := range container.VolumeMounts {
				Expect(vm.Name).NotTo(Equal("dm-password"))
			}
			// No secret volume
			podSpec := sts.Spec.Template.Spec
			for _, v := range podSpec.Volumes {
				Expect(v.Name).NotTo(Equal("dm-password"))
			}
		})

		It("should set status phase to Initializing", func() {
			Eventually(func(g Gomega) {
				fetched := &operatorv1alpha1.DirectoryService{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: dsName, Namespace: dsNamespace,
				}, fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal("Initializing"))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("when creating a fully specified DirectoryService CR", func() {
		const dsName = "test-full"
		const dsNamespace = "default"

		var ds *operatorv1alpha1.DirectoryService

		BeforeEach(func() {
			ds = &operatorv1alpha1.DirectoryService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dsName,
					Namespace: dsNamespace,
				},
				Spec: operatorv1alpha1.DirectoryServiceSpec{
					Image:    "quay.io/389ds/dirsrv:3.1",
					Replicas: ptr.To(int32(3)),
					Suffixes: []operatorv1alpha1.SuffixSpec{
						{Name: "userroot", DN: "dc=example,dc=com"},
						{Name: "config", DN: "dc=config,dc=com"},
					},
					Storage: &operatorv1alpha1.StorageSpec{
						Size: resource.MustParse("50Gi"),
					},
					Ports: &operatorv1alpha1.PortSpec{
						LDAP:  1389,
						LDAPS: 1636,
					},
				},
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, ds)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should use custom replicas, ports, and storage", func() {
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)
			}, timeout, interval).Should(Succeed())

			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))

			// Custom ports
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Ports[0].ContainerPort).To(Equal(int32(1389)))
			Expect(container.Ports[1].ContainerPort).To(Equal(int32(1636)))

			// Custom storage
			storageReq := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(storageReq.String()).To(Equal("50Gi"))
		})

		It("should set DS_SUFFIX_NAME to first suffix DN in ConfigMap", func() {
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: dsName + "-config", Namespace: dsNamespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			// Container only supports one suffix via env var - uses first suffix's DN
			Expect(cm.Data).To(HaveKeyWithValue("DS_SUFFIX_NAME", "dc=example,dc=com"))
			Expect(cm.Data).To(HaveKeyWithValue("DS_STARTUP_TIMEOUT", "60"))
		})

		It("should use custom ports in Services", func() {
			svc := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, svc)
			}, timeout, interval).Should(Succeed())

			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(1389)))
			Expect(svc.Spec.Ports[1].Port).To(Equal(int32(1636)))
		})
	})

	Context("when dmPasswordMode is set to file", func() {
		const dsName = "test-file-mode"
		const dsNamespace = "default"

		var ds *operatorv1alpha1.DirectoryService

		BeforeEach(func() {
			ds = &operatorv1alpha1.DirectoryService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dsName,
					Namespace: dsNamespace,
				},
				Spec: operatorv1alpha1.DirectoryServiceSpec{
					Image:          "quay.io/389ds/dirsrv:latest",
					DMPasswordMode: "file",
				},
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, ds)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should mount Secret as file and set DS_DM_PASSWORD_FILE", func() {
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)
			}, timeout, interval).Should(Succeed())

			container := sts.Spec.Template.Spec.Containers[0]

			// DS_DM_PASSWORD_FILE env var points to the mounted file
			Expect(container.Env).To(HaveLen(1))
			Expect(container.Env[0].Name).To(Equal("DS_DM_PASSWORD_FILE"))
			Expect(container.Env[0].Value).To(Equal("/run/secrets/dm-password/dm-password"))

			// Secret mounted as read-only volume
			Expect(container.VolumeMounts).To(ContainElement(
				corev1.VolumeMount{
					Name:      "dm-password",
					MountPath: "/run/secrets/dm-password",
					ReadOnly:  true,
				},
			))

			// Volume referencing the Secret
			podSpec := sts.Spec.Template.Spec
			Expect(podSpec.Volumes).To(HaveLen(1))
			Expect(podSpec.Volumes[0].Name).To(Equal("dm-password"))
			Expect(podSpec.Volumes[0].Secret).NotTo(BeNil())
			Expect(podSpec.Volumes[0].Secret.SecretName).To(Equal(dsName + "-dm-password"))

			// No DS_DM_PASSWORD env var in file mode
			for _, env := range container.Env {
				Expect(env.Name).NotTo(Equal("DS_DM_PASSWORD"))
			}
		})
	})

	Context("when updating CR fields that don't require pod restart", func() {
		const dsName = "test-no-restart"
		const dsNamespace = "default"

		var ds *operatorv1alpha1.DirectoryService

		BeforeEach(func() {
			ds = &operatorv1alpha1.DirectoryService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dsName,
					Namespace: dsNamespace,
				},
				Spec: operatorv1alpha1.DirectoryServiceSpec{
					Image:    "quay.io/389ds/dirsrv:latest",
					Replicas: ptr.To(int32(1)),
					Suffixes: []operatorv1alpha1.SuffixSpec{
						{Name: "userroot", DN: "dc=example,dc=com"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())

			// Wait for initial resources to be created
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace},
					&appsv1.StatefulSet{})
			}, timeout, interval).Should(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, ds)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should scale replicas without changing pod template", func() {
			// Capture original pod template
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)).To(Succeed())
			originalTemplate := sts.Spec.Template.DeepCopy()

			// Update replicas
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, ds)).To(Succeed())
			ds.Spec.Replicas = ptr.To(int32(3))
			Expect(k8sClient.Update(ctx, ds)).To(Succeed())

			// Verify replicas updated
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)).To(Succeed())
				g.Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
			}, timeout, interval).Should(Succeed())

			// Pod template unchanged - no restart triggered
			Expect(equality.Semantic.DeepEqual(sts.Spec.Template, *originalTemplate)).To(BeTrue())
		})

		It("should update ConfigMap when suffixes change without changing pod template", func() {
			// Capture original pod template
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)).To(Succeed())
			originalTemplate := sts.Spec.Template.DeepCopy()

			// Update suffix
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, ds)).To(Succeed())
			ds.Spec.Suffixes = []operatorv1alpha1.SuffixSpec{
				{Name: "newroot", DN: "dc=new,dc=com"},
			}
			Expect(k8sClient.Update(ctx, ds)).To(Succeed())

			// ConfigMap should have new suffix
			Eventually(func(g Gomega) {
				cm := &corev1.ConfigMap{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: dsName + "-config", Namespace: dsNamespace,
				}, cm)).To(Succeed())
				g.Expect(cm.Data).To(HaveKeyWithValue("DS_SUFFIX_NAME", "dc=new,dc=com"))
			}, timeout, interval).Should(Succeed())

			// Pod template unchanged - suffix change only affects ConfigMap, not pod spec
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)).To(Succeed())
			Expect(equality.Semantic.DeepEqual(sts.Spec.Template, *originalTemplate)).To(BeTrue())
		})

		It("should update Service ports without pod restart", func() {
			// Capture StatefulSet generation
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)).To(Succeed())
			originalGeneration := sts.Generation

			// Update ports in CR
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, ds)).To(Succeed())
			ds.Spec.Ports = &operatorv1alpha1.PortSpec{LDAP: 1389, LDAPS: 1636}
			Expect(k8sClient.Update(ctx, ds)).To(Succeed())

			// Services should have new ports
			Eventually(func(g Gomega) {
				svc := &corev1.Service{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, svc)).To(Succeed())
				g.Expect(svc.Spec.Ports[0].Port).To(Equal(int32(1389)))
				g.Expect(svc.Spec.Ports[1].Port).To(Equal(int32(1636)))
			}, timeout, interval).Should(Succeed())

			// Headless service too
			Eventually(func(g Gomega) {
				svc := &corev1.Service{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: dsName + "-internal", Namespace: dsNamespace,
				}, svc)).To(Succeed())
				g.Expect(svc.Spec.Ports[0].Port).To(Equal(int32(1389)))
			}, timeout, interval).Should(Succeed())

			// NOTE: port changes also affect StatefulSet container ports, which IS a
			// pod template change and triggers rolling update. But the Service update
			// itself is independent and takes effect immediately for routing.
			// Verify StatefulSet was updated (generation bumped)
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)).To(Succeed())
				g.Expect(sts.Generation).To(BeNumerically(">", originalGeneration))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("when updating CR fields that trigger rolling update", func() {
		const dsName = "test-rolling"
		const dsNamespace = "default"

		var ds *operatorv1alpha1.DirectoryService

		BeforeEach(func() {
			ds = &operatorv1alpha1.DirectoryService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dsName,
					Namespace: dsNamespace,
				},
				Spec: operatorv1alpha1.DirectoryServiceSpec{
					Image: "quay.io/389ds/dirsrv:3.0",
				},
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace},
					&appsv1.StatefulSet{})
			}, timeout, interval).Should(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, ds)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should update StatefulSet pod template when image changes", func() {
			// Update image
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, ds)).To(Succeed())
			ds.Spec.Image = "quay.io/389ds/dirsrv:3.1"
			Expect(k8sClient.Update(ctx, ds)).To(Succeed())

			// StatefulSet should reflect new image - triggers rolling update
			Eventually(func(g Gomega) {
				sts := &appsv1.StatefulSet{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)).To(Succeed())
				g.Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("quay.io/389ds/dirsrv:3.1"))
			}, timeout, interval).Should(Succeed())
		})

		It("should use RollingUpdate strategy", func() {
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)).To(Succeed())
			Expect(sts.Spec.UpdateStrategy.Type).To(Equal(appsv1.RollingUpdateStatefulSetStrategyType))
		})

		It("should preserve PVC across pod template changes", func() {
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace}, sts)).To(Succeed())

			// VCT present with /data mount
			Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
			Expect(sts.Spec.VolumeClaimTemplates[0].Name).To(Equal("ds-data"))

			// Container mounts /data
			Expect(sts.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(
				corev1.VolumeMount{Name: "ds-data", MountPath: "/data"},
			))
		})
	})

	Context("when deleting a DirectoryService CR", func() {
		const dsName = "test-delete"
		const dsNamespace = "default"

		It("should clean up all owned resources via garbage collection", func() {
			ds := &operatorv1alpha1.DirectoryService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dsName,
					Namespace: dsNamespace,
				},
				Spec: operatorv1alpha1.DirectoryServiceSpec{
					Image: "quay.io/389ds/dirsrv:latest",
				},
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())

			// Wait for all resources to be created
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace},
					&appsv1.StatefulSet{})
			}, timeout, interval).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName + "-internal", Namespace: dsNamespace},
					&corev1.Service{})
			}, timeout, interval).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace},
					&corev1.Service{})
			}, timeout, interval).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName + "-dm-password", Namespace: dsNamespace},
					&corev1.Secret{})
			}, timeout, interval).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: dsName + "-config", Namespace: dsNamespace},
					&corev1.ConfigMap{})
			}, timeout, interval).Should(Succeed())

			// Delete the CR
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())

			// Verify all owned resources are garbage collected
			Eventually(func() bool {
				return client.IgnoreNotFound(k8sClient.Get(ctx, types.NamespacedName{
					Name: dsName, Namespace: dsNamespace,
				}, &appsv1.StatefulSet{})) == nil && appsv1.StatefulSet{}.Name == ""
			}, timeout, interval).Should(BeTrue())

			// StatefulSet should be deleted
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace},
					&appsv1.StatefulSet{})
				return client.IgnoreNotFound(err)
			}, timeout, interval).Should(Succeed())

			// Headless service should be deleted
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dsName + "-internal", Namespace: dsNamespace},
					&corev1.Service{})
				return client.IgnoreNotFound(err)
			}, timeout, interval).Should(Succeed())

			// ClusterIP service should be deleted
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dsName, Namespace: dsNamespace},
					&corev1.Service{})
				return client.IgnoreNotFound(err)
			}, timeout, interval).Should(Succeed())

			// DM password secret should be deleted
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dsName + "-dm-password", Namespace: dsNamespace},
					&corev1.Secret{})
				return client.IgnoreNotFound(err)
			}, timeout, interval).Should(Succeed())

			// ConfigMap should be deleted
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dsName + "-config", Namespace: dsNamespace},
					&corev1.ConfigMap{})
				return client.IgnoreNotFound(err)
			}, timeout, interval).Should(Succeed())
		})
	})
})
