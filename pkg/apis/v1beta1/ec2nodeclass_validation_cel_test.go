/*
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

package v1beta1_test

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/resource"
	karpv1beta1 "sigs.k8s.io/karpenter/pkg/apis/v1beta1"

	"github.com/aws/karpenter-provider-aws/pkg/apis/v1beta1"
	"github.com/aws/karpenter-provider-aws/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CEL/Validation", func() {
	var nc *v1beta1.EC2NodeClass

	BeforeEach(func() {
		if env.Version.Minor() < 25 {
			Skip("CEL Validation is for 1.25>")
		}
		nc = test.BetaEC2NodeClass()
	})
	It("should succeed if just specifying role", func() {
		Expect(env.Client.Create(ctx, nc)).To(Succeed())
	})
	It("should succeed if just specifying instance profile", func() {
		nc.Spec.InstanceProfile = lo.ToPtr("test-instance-profile")
		nc.Spec.Role = ""
		Expect(env.Client.Create(ctx, nc)).To(Succeed())
	})
	It("should fail if specifying both instance profile and role", func() {
		nc.Spec.InstanceProfile = lo.ToPtr("test-instance-profile")
		Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
	})
	It("should fail if not specifying one of instance profile and role", func() {
		nc.Spec.Role = ""
		Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
	})
	Context("UserData", func() {
		It("should succeed if user data is empty", func() {
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
	})
	Context("Tags", func() {
		It("should succeed when tags are empty", func() {
			nc.Spec.Tags = map[string]string{}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should succeed if tags aren't in restricted tag keys", func() {
			nc.Spec.Tags = map[string]string{
				"karpenter.sh/custom-key": "value",
				"karpenter.sh/managed":    "true",
				"kubernetes.io/role/key":  "value",
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should fail if tags contain a restricted domain key", func() {
			nc.Spec.Tags = map[string]string{
				karpv1beta1.NodePoolLabelKey: "value",
			}
			Expect(env.Client.Create(ctx, nc)).To(Not(Succeed()))
			nc.Spec.Tags = map[string]string{
				"kubernetes.io/cluster/test": "value",
			}
			Expect(env.Client.Create(ctx, nc)).To(Not(Succeed()))
			nc.Spec.Tags = map[string]string{
				karpv1beta1.ManagedByAnnotationKey: "test",
			}
			Expect(env.Client.Create(ctx, nc)).To(Not(Succeed()))
			nc.Spec.Tags = map[string]string{
				v1beta1.LabelNodeClass: "test",
			}
			Expect(env.Client.Create(ctx, nc)).To(Not(Succeed()))
			nc.Spec.Tags = map[string]string{
				"karpenter.sh/nodeclaim": "test",
			}
			Expect(env.Client.Create(ctx, nc)).To(Not(Succeed()))
		})
	})
	Context("SubnetSelectorTerms", func() {
		It("should succeed with a valid subnet selector on tags", func() {
			nc.Spec.SubnetSelectorTerms = []v1beta1.SubnetSelectorTerm{
				{
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should succeed with a valid subnet selector on id", func() {
			nc.Spec.SubnetSelectorTerms = []v1beta1.SubnetSelectorTerm{
				{
					ID: "subnet-12345749",
				},
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should fail when subnet selector terms is set to nil", func() {
			nc.Spec.SubnetSelectorTerms = nil
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when no subnet selector terms exist", func() {
			nc.Spec.SubnetSelectorTerms = []v1beta1.SubnetSelectorTerm{}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a subnet selector term has no values", func() {
			nc.Spec.SubnetSelectorTerms = []v1beta1.SubnetSelectorTerm{
				{},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a subnet selector term has no tag map values", func() {
			nc.Spec.SubnetSelectorTerms = []v1beta1.SubnetSelectorTerm{
				{
					Tags: map[string]string{},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a subnet selector term has a tag map key that is empty", func() {
			nc.Spec.SubnetSelectorTerms = []v1beta1.SubnetSelectorTerm{
				{
					Tags: map[string]string{
						"test": "",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a subnet selector term has a tag map value that is empty", func() {
			nc.Spec.SubnetSelectorTerms = []v1beta1.SubnetSelectorTerm{
				{
					Tags: map[string]string{
						"": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when the last subnet selector is invalid", func() {
			nc.Spec.SubnetSelectorTerms = []v1beta1.SubnetSelectorTerm{
				{
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
				{
					Tags: map[string]string{
						"test2": "testvalue2",
					},
				},
				{
					Tags: map[string]string{
						"test3": "testvalue3",
					},
				},
				{
					Tags: map[string]string{
						"": "testvalue4",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when specifying id with tags", func() {
			nc.Spec.SubnetSelectorTerms = []v1beta1.SubnetSelectorTerm{
				{
					ID: "subnet-12345749",
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
	})
	Context("SecurityGroupSelectorTerms", func() {
		It("should succeed with a valid security group selector on tags", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should succeed with a valid security group selector on id", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{
					ID: "sg-12345749",
				},
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should succeed with a valid security group selector on name", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{
					Name: "testname",
				},
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should fail when security group selector terms is set to nil", func() {
			nc.Spec.SecurityGroupSelectorTerms = nil
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when no security group selector terms exist", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a security group selector term has no values", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a security group selector term has no tag map values", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{
					Tags: map[string]string{},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a security group selector term has a tag map key that is empty", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{
					Tags: map[string]string{
						"test": "",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a security group selector term has a tag map value that is empty", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{
					Tags: map[string]string{
						"": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when the last security group selector is invalid", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
				{
					Tags: map[string]string{
						"test2": "testvalue2",
					},
				},
				{
					Tags: map[string]string{
						"test3": "testvalue3",
					},
				},
				{
					Tags: map[string]string{
						"": "testvalue4",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when specifying id with tags", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{
					ID: "sg-12345749",
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when specifying id with name", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{
					ID:   "sg-12345749",
					Name: "my-security-group",
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when specifying name with tags", func() {
			nc.Spec.SecurityGroupSelectorTerms = []v1beta1.SecurityGroupSelectorTerm{
				{
					Name: "my-security-group",
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
	})
	Context("AMISelectorTerms", func() {
		It("should succeed with a valid ami selector on tags", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should succeed with a valid ami selector on id", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					ID: "ami-12345749",
				},
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should succeed with a valid ami selector on name", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					Name: "testname",
				},
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should succeed with a valid ami selector on name and owner", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					Name:  "testname",
					Owner: "testowner",
				},
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should succeed when an ami selector term has an owner key with tags", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					Owner: "testowner",
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should fail when a ami selector term has no values", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a ami selector term has no tag map values", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					Tags: map[string]string{},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a ami selector term has a tag map key that is empty", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					Tags: map[string]string{
						"test": "",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when a ami selector term has a tag map value that is empty", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					Tags: map[string]string{
						"": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when the last ami selector is invalid", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
				{
					Tags: map[string]string{
						"test2": "testvalue2",
					},
				},
				{
					Tags: map[string]string{
						"test3": "testvalue3",
					},
				},
				{
					Tags: map[string]string{
						"": "testvalue4",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when specifying id with tags", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					ID: "ami-12345749",
					Tags: map[string]string{
						"test": "testvalue",
					},
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when specifying id with name", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					ID:   "ami-12345749",
					Name: "my-custom-ami",
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when specifying id with owner", func() {
			nc.Spec.AMISelectorTerms = []v1beta1.AMISelectorTerm{
				{
					ID:    "ami-12345749",
					Owner: "123456789",
				},
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when AMIFamily is Custom and not AMISelectorTerms", func() {
			nc.Spec.AMIFamily = &v1beta1.AMIFamilyCustom
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
	})
	Context("MetadataOptions", func() {
		It("should succeed for valid inputs", func() {
			nc.Spec.MetadataOptions = &v1beta1.MetadataOptions{
				HTTPEndpoint:            aws.String("disabled"),
				HTTPProtocolIPv6:        aws.String("enabled"),
				HTTPPutResponseHopLimit: aws.Int64(34),
				HTTPTokens:              aws.String("optional"),
			}
			Expect(env.Client.Create(ctx, nc)).To(Succeed())
		})
		It("should fail for invalid for HTTPEndpoint", func() {
			nc.Spec.MetadataOptions = &v1beta1.MetadataOptions{
				HTTPEndpoint: aws.String("test"),
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail for invalid for HTTPProtocolIPv6", func() {
			nc.Spec.MetadataOptions = &v1beta1.MetadataOptions{
				HTTPProtocolIPv6: aws.String("test"),
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail for invalid for HTTPPutResponseHopLimit", func() {
			nc.Spec.MetadataOptions = &v1beta1.MetadataOptions{
				HTTPPutResponseHopLimit: aws.Int64(-5),
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail for invalid for HTTPTokens", func() {
			nc.Spec.MetadataOptions = &v1beta1.MetadataOptions{
				HTTPTokens: aws.String("test"),
			}
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
	})
	Context("BlockDeviceMappings", func() {
		It("should succeed if more than one root volume is specified", func() {
			nodeClass := test.BetaEC2NodeClass(v1beta1.EC2NodeClass{
				Spec: v1beta1.EC2NodeClassSpec{
					BlockDeviceMappings: []*v1beta1.BlockDeviceMapping{
						{
							DeviceName: aws.String("map-device-1"),
							EBS: &v1beta1.BlockDevice{
								VolumeSize: resource.NewScaledQuantity(500, resource.Giga),
							},

							RootVolume: true,
						},
						{
							DeviceName: aws.String("map-device-2"),
							EBS: &v1beta1.BlockDevice{
								VolumeSize: resource.NewScaledQuantity(50, resource.Tera),
							},

							RootVolume: false,
						},
					},
				},
			})
			Expect(env.Client.Create(ctx, nodeClass)).To(Succeed())
		})
		It("should succeed for valid VolumeSize in G", func() {
			nodeClass := test.BetaEC2NodeClass(v1beta1.EC2NodeClass{
				Spec: v1beta1.EC2NodeClassSpec{
					BlockDeviceMappings: []*v1beta1.BlockDeviceMapping{
						{
							DeviceName: aws.String("map-device-1"),
							EBS: &v1beta1.BlockDevice{
								VolumeSize: resource.NewScaledQuantity(58, resource.Giga),
							},
							RootVolume: false,
						},
					},
				},
			})
			Expect(env.Client.Create(ctx, nodeClass)).To(Succeed())
		})
		It("should succeed for valid VolumeSize in T", func() {
			nodeClass := test.BetaEC2NodeClass(v1beta1.EC2NodeClass{
				Spec: v1beta1.EC2NodeClassSpec{
					BlockDeviceMappings: []*v1beta1.BlockDeviceMapping{
						{
							DeviceName: aws.String("map-device-1"),
							EBS: &v1beta1.BlockDevice{
								VolumeSize: resource.NewScaledQuantity(45, resource.Tera),
							},
							RootVolume: false,
						},
					},
				},
			})
			Expect(env.Client.Create(ctx, nodeClass)).To(Succeed())
		})
		It("should fail if more than one root volume is specified", func() {
			nodeClass := test.BetaEC2NodeClass(v1beta1.EC2NodeClass{
				Spec: v1beta1.EC2NodeClassSpec{
					BlockDeviceMappings: []*v1beta1.BlockDeviceMapping{
						{
							DeviceName: aws.String("map-device-1"),
							EBS: &v1beta1.BlockDevice{
								VolumeSize: resource.NewScaledQuantity(50, resource.Giga),
							},
							RootVolume: true,
						},
						{
							DeviceName: aws.String("map-device-2"),
							EBS: &v1beta1.BlockDevice{
								VolumeSize: resource.NewScaledQuantity(50, resource.Giga),
							},
							RootVolume: true,
						},
					},
				},
			})
			Expect(env.Client.Create(ctx, nodeClass)).To(Not(Succeed()))
		})
		It("should fail VolumeSize is less then 1Gi/1G", func() {
			nodeClass := test.BetaEC2NodeClass(v1beta1.EC2NodeClass{
				Spec: v1beta1.EC2NodeClassSpec{
					BlockDeviceMappings: []*v1beta1.BlockDeviceMapping{
						{
							DeviceName: aws.String("map-device-1"),
							EBS: &v1beta1.BlockDevice{
								VolumeSize: resource.NewScaledQuantity(1, resource.Milli),
							},
							RootVolume: false,
						},
					},
				},
			})
			Expect(env.Client.Create(ctx, nodeClass)).To(Not(Succeed()))
		})
		It("should fail VolumeSize is greater then 64T", func() {
			nodeClass := test.BetaEC2NodeClass(v1beta1.EC2NodeClass{
				Spec: v1beta1.EC2NodeClassSpec{
					BlockDeviceMappings: []*v1beta1.BlockDeviceMapping{
						{
							DeviceName: aws.String("map-device-1"),
							EBS: &v1beta1.BlockDevice{
								VolumeSize: resource.NewScaledQuantity(100, resource.Tera),
							},
							RootVolume: false,
						},
					},
				},
			})
			Expect(env.Client.Create(ctx, nodeClass)).To(Not(Succeed()))
		})
		It("should fail for VolumeSize that do not parse into quantity values", func() {
			nodeClass := test.BetaEC2NodeClass(v1beta1.EC2NodeClass{
				Spec: v1beta1.EC2NodeClassSpec{
					BlockDeviceMappings: []*v1beta1.BlockDeviceMapping{
						{
							DeviceName: aws.String("map-device-1"),
							EBS: &v1beta1.BlockDevice{
								VolumeSize: &resource.Quantity{},
							},
							RootVolume: false,
						},
					},
				},
			})
			Expect(env.Client.Create(ctx, nodeClass)).To(Not(Succeed()))
		})
	})
	Context("Role Immutability", func() {
		It("should fail if role is not defined", func() {
			nc.Spec.Role = ""
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail when updating the role", func() {
			nc.Spec.Role = "test-role"
			Expect(env.Client.Create(ctx, nc)).To(Succeed())

			nc.Spec.Role = "test-role2"
			Expect(env.Client.Create(ctx, nc)).ToNot(Succeed())
		})
		It("should fail to switch between an unmanaged and managed instance profile", func() {
			nc.Spec.Role = ""
			nc.Spec.InstanceProfile = lo.ToPtr("test-instance-profile")
			Expect(env.Client.Create(ctx, nc)).To(Succeed())

			nc.Spec.Role = "test-role"
			nc.Spec.InstanceProfile = nil
			Expect(env.Client.Update(ctx, nc)).ToNot(Succeed())
		})
		It("should fail to switch between a managed and unmanaged instance profile", func() {
			nc.Spec.Role = "test-role"
			nc.Spec.InstanceProfile = nil
			Expect(env.Client.Create(ctx, nc)).To(Succeed())

			nc.Spec.Role = ""
			nc.Spec.InstanceProfile = lo.ToPtr("test-instance-profile")
			Expect(env.Client.Update(ctx, nc)).ToNot(Succeed())
		})
	})
})
