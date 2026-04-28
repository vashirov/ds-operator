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

package v1alpha1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/389ds/ds-operator/api/v1alpha1"
)

var _ = Describe("DirectoryService CRD Validation", func() {

	// validDS returns a minimal valid DirectoryService for use as a base in tests.
	// Each test should set a unique name before creating.
	validDS := func(name string) *operatorv1alpha1.DirectoryService {
		return &operatorv1alpha1.DirectoryService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: operatorv1alpha1.DirectoryServiceSpec{
				Image: "quay.io/389ds/dirsrv:latest",
			},
		}
	}

	Context("valid CRs", func() {
		It("should accept a minimal CR with only image", func() {
			ds := validDS("valid-minimal")
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})

		It("should accept a fully specified CR", func() {
			ds := validDS("valid-full")
			ds.Spec.Replicas = ptr.To(int32(3))
			ds.Spec.Suffixes = []operatorv1alpha1.SuffixSpec{
				{Name: "userroot", DN: "dc=example,dc=com", CreateEntries: true},
			}
			ds.Spec.Storage = &operatorv1alpha1.StorageSpec{
				Size: resource.MustParse("10Gi"),
			}
			ds.Spec.Ports = &operatorv1alpha1.PortSpec{
				LDAP:  3389,
				LDAPS: 3636,
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})

		It("should apply default values for replicas and ports", func() {
			ds := validDS("valid-defaults")
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())

			fetched := &operatorv1alpha1.DirectoryService{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ds), fetched)).To(Succeed())
			Expect(*fetched.Spec.Replicas).To(Equal(int32(1)))
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})
	})

	Context("image field validation", func() {
		It("should reject a CR with empty image", func() {
			ds := validDS("invalid-empty-image")
			ds.Spec.Image = ""
			err := k8sClient.Create(ctx, ds)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("image"))
		})
	})

	Context("replicas validation", func() {
		It("should reject replicas = 0", func() {
			ds := validDS("invalid-replicas-zero")
			ds.Spec.Replicas = ptr.To(int32(0))
			err := k8sClient.Create(ctx, ds)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.replicas"))
		})

		It("should reject negative replicas", func() {
			ds := validDS("invalid-replicas-neg")
			ds.Spec.Replicas = ptr.To(int32(-1))
			err := k8sClient.Create(ctx, ds)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.replicas"))
		})

		It("should accept replicas = 1", func() {
			ds := validDS("valid-replicas-one")
			ds.Spec.Replicas = ptr.To(int32(1))
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})
	})

	Context("suffix validation", func() {
		It("should reject suffix name starting with a digit", func() {
			ds := validDS("invalid-suffix-digit")
			ds.Spec.Suffixes = []operatorv1alpha1.SuffixSpec{
				{Name: "123bad", DN: "dc=example,dc=com"},
			}
			err := k8sClient.Create(ctx, ds)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.suffixes[0].name"))
		})

		It("should reject suffix name with special characters", func() {
			ds := validDS("invalid-suffix-special")
			ds.Spec.Suffixes = []operatorv1alpha1.SuffixSpec{
				{Name: "user@root", DN: "dc=example,dc=com"},
			}
			err := k8sClient.Create(ctx, ds)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.suffixes[0].name"))
		})

		It("should reject suffix without DN", func() {
			ds := validDS("invalid-suffix-no-dn")
			ds.Spec.Suffixes = []operatorv1alpha1.SuffixSpec{
				{Name: "userroot"},
			}
			err := k8sClient.Create(ctx, ds)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dn"))
		})

		It("should accept suffix with hyphens and underscores", func() {
			ds := validDS("valid-suffix-chars")
			ds.Spec.Suffixes = []operatorv1alpha1.SuffixSpec{
				{Name: "user-root_01", DN: "dc=example,dc=com"},
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})

		It("should accept multiple suffixes", func() {
			ds := validDS("valid-multi-suffix")
			ds.Spec.Suffixes = []operatorv1alpha1.SuffixSpec{
				{Name: "userroot", DN: "dc=example,dc=com"},
				{Name: "serviceroot", DN: "dc=services,dc=example,dc=com"},
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})
	})

	Context("port validation", func() {
		DescribeTable("should reject ports out of valid range",
			func(ldap, ldaps int32, fieldSubstring string) {
				ds := validDS(fmt.Sprintf("invalid-port-%d-%d", ldap, ldaps))
				ds.Spec.Ports = &operatorv1alpha1.PortSpec{
					LDAP:  ldap,
					LDAPS: ldaps,
				}
				err := k8sClient.Create(ctx, ds)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(fieldSubstring))
			},
			Entry("ldap port exceeds max", int32(70000), int32(3636), "spec.ports.ldap"),
			Entry("ldaps port exceeds max", int32(3389), int32(99999), "spec.ports.ldaps"),
		)

		// Note: port = 0 with omitempty is treated as absent by JSON serialization,
		// so the kubebuilder default (3389/3636) applies and validation passes.
		// This is correct behavior — zero means "use default".
		It("should apply default when port is 0 (omitempty)", func() {
			ds := validDS("valid-port-zero-default")
			ds.Spec.Ports = &operatorv1alpha1.PortSpec{
				LDAP:  0,
				LDAPS: 0,
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())

			fetched := &operatorv1alpha1.DirectoryService{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ds), fetched)).To(Succeed())
			Expect(fetched.Spec.Ports.LDAP).To(Equal(int32(3389)))
			Expect(fetched.Spec.Ports.LDAPS).To(Equal(int32(3636)))
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})

		It("should accept valid port numbers", func() {
			ds := validDS("valid-ports")
			ds.Spec.Ports = &operatorv1alpha1.PortSpec{
				LDAP:  1389,
				LDAPS: 1636,
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})

		It("should accept boundary port values", func() {
			ds := validDS("valid-ports-boundary")
			ds.Spec.Ports = &operatorv1alpha1.PortSpec{
				LDAP:  1,
				LDAPS: 65535,
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})
	})

	Context("storage validation", func() {
		It("should accept storage with explicit size", func() {
			ds := validDS("valid-storage")
			ds.Spec.Storage = &operatorv1alpha1.StorageSpec{
				Size: resource.MustParse("50Gi"),
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})

		It("should accept storage with storageClassName", func() {
			ds := validDS("valid-storage-class")
			ds.Spec.Storage = &operatorv1alpha1.StorageSpec{
				Size:             resource.MustParse("10Gi"),
				StorageClassName: ptr.To("premium-ssd"),
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})
	})

	Context("dmPasswordMode validation", func() {
		It("should default to env mode", func() {
			ds := validDS("valid-dm-default")
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())

			fetched := &operatorv1alpha1.DirectoryService{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ds), fetched)).To(Succeed())
			Expect(fetched.Spec.DMPasswordMode).To(Equal("env"))
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})

		It("should accept env mode", func() {
			ds := validDS("valid-dm-env")
			ds.Spec.DMPasswordMode = "env"
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})

		It("should accept file mode", func() {
			ds := validDS("valid-dm-file")
			ds.Spec.DMPasswordMode = "file"
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ds)).To(Succeed())
		})

		It("should reject invalid mode", func() {
			ds := validDS("invalid-dm-mode")
			ds.Spec.DMPasswordMode = "invalid"
			err := k8sClient.Create(ctx, ds)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dmPasswordMode"))
		})
	})
})
