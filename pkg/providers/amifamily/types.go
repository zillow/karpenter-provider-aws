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

package amifamily

import (
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
)

const (
	// AMIVersionLatest is the version used in EKS aliases to represent the latest version. This maps to different
	// values in the SSM path, depending on the AMI type (e.g. "recommended" for AL2/AL2023)).
	AMIVersionLatest = "latest"
)

type AMI struct {
	Name         string
	AmiID        string
	CreationDate string
	Requirements scheduling.Requirements
}

type AMIs []AMI

// Sort orders the AMIs by creation date in descending order.
// If creation date is nil or two AMIs have the same creation date, the AMIs will be sorted by ID, which is guaranteed to be unique, in ascending order.
func (a AMIs) Sort() {
	sort.Slice(a, func(i, j int) bool {
		itime, _ := time.Parse(time.RFC3339, a[i].CreationDate)
		jtime, _ := time.Parse(time.RFC3339, a[j].CreationDate)
		if itime.Unix() != jtime.Unix() {
			return itime.Unix() > jtime.Unix()
		}
		return a[i].AmiID < a[j].AmiID
	})
}

type Variant string

var (
	VariantStandard Variant = "standard"
	VariantNvidia   Variant = "nvidia"
	VariantNeuron   Variant = "neuron"
)

func NewVariant(v string) (Variant, error) {
	var wellKnownVariants = sets.New(VariantStandard, VariantNvidia, VariantNeuron)
	variant := Variant(v)
	if !wellKnownVariants.Has(variant) {
		return variant, fmt.Errorf("%q is not a well-known variant", variant)
	}
	return variant, nil
}

func (v Variant) Requirements() scheduling.Requirements {
	switch v {
	case VariantStandard:
		return scheduling.NewRequirements(
			scheduling.NewRequirement(v1.LabelInstanceAcceleratorCount, corev1.NodeSelectorOpDoesNotExist),
			scheduling.NewRequirement(v1.LabelInstanceGPUCount, corev1.NodeSelectorOpDoesNotExist),
		)
	case VariantNvidia:
		return scheduling.NewRequirements(scheduling.NewRequirement(v1.LabelInstanceAcceleratorCount, corev1.NodeSelectorOpExists))
	case VariantNeuron:
		return scheduling.NewRequirements(scheduling.NewRequirement(v1.LabelInstanceGPUCount, corev1.NodeSelectorOpExists))
	}
	return nil
}

type DescribeImageQuery struct {
	Filters []*ec2.Filter
	Owners  []string
	// KnownRequirements is a map from image IDs to a set of known requirements.
	// When discovering image IDs via SSM we know additional requirements which aren't surfaced by ec2:DescribeImage (e.g. GPU / Neuron compatibility)
	// Sometimes, an image may have multiple sets of known requirements. For example, the AL2 GPU AMI is compatible with both Neuron and Nvidia GPU
	// instances, which means we need a set of requirements for either instance type.
	KnownRequirements map[string][]scheduling.Requirements
}

func (q DescribeImageQuery) DescribeImagesInput() *ec2.DescribeImagesInput {
	return &ec2.DescribeImagesInput{
		// Don't include filters in the Describe Images call as EC2 API doesn't allow empty filters.
		Filters:    lo.Ternary(len(q.Filters) > 0, q.Filters, nil),
		Owners:     lo.Ternary(len(q.Owners) > 0, lo.ToSlicePtr(q.Owners), nil),
		MaxResults: aws.Int64(1000),
	}
}

func (q DescribeImageQuery) RequirementsForImageWithArchitecture(image string, arch string) []scheduling.Requirements {
	if knownRequirements, ok := q.KnownRequirements[image]; ok {
		return lo.Map(knownRequirements, func(r scheduling.Requirements, _ int) scheduling.Requirements {
			r.Add(scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, arch))
			return r
		})
	}
	return []scheduling.Requirements{scheduling.NewRequirements(scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, arch))}
}
