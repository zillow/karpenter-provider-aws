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

package instanceprofile

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	awserrors "github.com/aws/karpenter-provider-aws/pkg/errors"
	"github.com/aws/karpenter-provider-aws/pkg/operator/options"
)

// ResourceOwner is an object that manages an instance profile
type ResourceOwner interface {
	GetUID() types.UID
	InstanceProfileName(string, string) string
	InstanceProfileRole() string
	InstanceProfileTags(string) map[string]string
}

type Provider interface {
	Create(context.Context, ResourceOwner) (string, error)
	Delete(context.Context, ResourceOwner) error
}

type DefaultProvider struct {
	region string
	iamapi iamiface.IAMAPI
	cache  *cache.Cache
}

func NewDefaultProvider(region string, iamapi iamiface.IAMAPI, cache *cache.Cache) *DefaultProvider {
	return &DefaultProvider{
		region: region,
		iamapi: iamapi,
		cache:  cache,
	}
}

func (p *DefaultProvider) Create(ctx context.Context, m ResourceOwner) (string, error) {
	profileName := m.InstanceProfileName(options.FromContext(ctx).ClusterName, p.region)
	tags := lo.Assign(m.InstanceProfileTags(options.FromContext(ctx).ClusterName), map[string]string{corev1.LabelTopologyRegion: p.region})

	// An instance profile exists for this NodeClass
	if _, ok := p.cache.Get(string(m.GetUID())); ok {
		return profileName, nil
	}
	// Validate if the instance profile exists and has the correct role assigned to it
	var instanceProfile *iam.InstanceProfile
	out, err := p.iamapi.GetInstanceProfileWithContext(ctx, &iam.GetInstanceProfileInput{InstanceProfileName: aws.String(profileName)})
	if err != nil {
		if !awserrors.IsNotFound(err) {
			return "", fmt.Errorf("getting instance profile %q, %w", profileName, err)
		}
		o, err := p.iamapi.CreateInstanceProfileWithContext(ctx, &iam.CreateInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			Tags:                lo.MapToSlice(tags, func(k, v string) *iam.Tag { return &iam.Tag{Key: aws.String(k), Value: aws.String(v)} }),
		})
		if err != nil {
			return "", fmt.Errorf("creating instance profile %q, %w", profileName, err)
		}
		instanceProfile = o.InstanceProfile
	} else {
		if !lo.ContainsBy(out.InstanceProfile.Tags, func(t *iam.Tag) bool {
			return lo.FromPtr(t.Key) == v1.EKSClusterNameTagKey
		}) {
			if _, err = p.iamapi.TagInstanceProfileWithContext(ctx, &iam.TagInstanceProfileInput{
				InstanceProfileName: aws.String(profileName),
				Tags:                lo.MapToSlice(tags, func(k, v string) *iam.Tag { return &iam.Tag{Key: aws.String(k), Value: aws.String(v)} }),
			}); err != nil {
				return "", fmt.Errorf("tagging instance profile %q, %w", profileName, err)
			}
		}
		instanceProfile = out.InstanceProfile
	}
	// Instance profiles can only have a single role assigned to them so this profile either has 1 or 0 roles
	// https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html
	if len(instanceProfile.Roles) == 1 {
		if aws.StringValue(instanceProfile.Roles[0].RoleName) == m.InstanceProfileRole() {
			return profileName, nil
		}
		if _, err = p.iamapi.RemoveRoleFromInstanceProfileWithContext(ctx, &iam.RemoveRoleFromInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			RoleName:            instanceProfile.Roles[0].RoleName,
		}); err != nil {
			return "", fmt.Errorf("removing role %q for instance profile %q, %w", aws.StringValue(instanceProfile.Roles[0].RoleName), profileName, err)
		}
	}
	if _, err = p.iamapi.AddRoleToInstanceProfileWithContext(ctx, &iam.AddRoleToInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
		RoleName:            aws.String(m.InstanceProfileRole()),
	}); err != nil {
		return "", fmt.Errorf("adding role %q to instance profile %q, %w", m.InstanceProfileRole(), profileName, err)
	}
	p.cache.SetDefault(string(m.GetUID()), nil)
	return aws.StringValue(instanceProfile.InstanceProfileName), nil
}

func (p *DefaultProvider) Delete(ctx context.Context, m ResourceOwner) error {
	profileName := m.InstanceProfileName(options.FromContext(ctx).ClusterName, p.region)
	out, err := p.iamapi.GetInstanceProfileWithContext(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})
	if err != nil {
		return awserrors.IgnoreNotFound(fmt.Errorf("getting instance profile %q, %w", profileName, err))
	}
	// Instance profiles can only have a single role assigned to them so this profile either has 1 or 0 roles
	// https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html
	if len(out.InstanceProfile.Roles) == 1 {
		if _, err = p.iamapi.RemoveRoleFromInstanceProfileWithContext(ctx, &iam.RemoveRoleFromInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			RoleName:            out.InstanceProfile.Roles[0].RoleName,
		}); err != nil {
			return fmt.Errorf("removing role %q from instance profile %q, %w", aws.StringValue(out.InstanceProfile.Roles[0].RoleName), profileName, err)
		}
	}
	if _, err = p.iamapi.DeleteInstanceProfileWithContext(ctx, &iam.DeleteInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	}); err != nil {
		return awserrors.IgnoreNotFound(fmt.Errorf("deleting instance profile %q, %w", profileName, err))
	}
	return nil
}
