package cloudresource

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"internal-developer-platform/src/shared/errors"
)

type GenericKCCManifest struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   KCCMetadata    `yaml:"metadata"`
	Spec       map[string]any `yaml:"spec"`
}

type KCCMetadata struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace,omitempty"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

func ValidateMetadata(meta ResourceMetadata) error {
	if strings.TrimSpace(meta.RequestID) == "" {
		return errors.NewValidationError(ErrMsgRequestIDMandatory)
	}
	if strings.TrimSpace(meta.Owner) == "" {
		return errors.NewValidationError(ErrMsgOwnerMandatory)
	}
	if strings.TrimSpace(meta.CostCenter) == "" {
		return errors.NewValidationError(ErrMsgCostCenterMandatory)
	}
	if strings.TrimSpace(meta.WorkloadID) == "" {
		return errors.NewValidationError(ErrMsgWorkloadIDMandatory)
	}
	if strings.TrimSpace(meta.Environment) == "" {
		return errors.NewValidationError(ErrMsgEnvironmentMandatory)
	}
	return nil
}

func ValidateIAMRole(role string) error {
	if ProhibitedIAMRoles[role] {
		return errors.NewForbiddenError(fmt.Sprintf(ErrMsgProhibitedRoleFmt, role))
	}
	if !PermittedIAMRoles[role] {
		return errors.NewForbiddenError(fmt.Sprintf(ErrMsgUnauthorizedRoleFmt, role))
	}
	return nil
}

func buildLabels(meta ResourceMetadata) map[string]string {
	return map[string]string{
		LabelRequestID:   meta.RequestID,
		LabelOwner:       meta.Owner,
		LabelCostCenter:  meta.CostCenter,
		LabelWorkloadID:  meta.WorkloadID,
		LabelEnvironment: meta.Environment,
		LabelManagedBy:   ManagedByPlatformValue,
	}
}

func GenerateWorkloadIdentityManifest(spec WorkloadIdentityBindingSpec) (string, error) {
	if err := ValidateMetadata(spec.Metadata); err != nil {
		return "", err
	}
	if spec.GSAEmail == "" || spec.KSAName == "" || spec.Namespace == "" || spec.ProjectID == "" {
		return "", errors.NewValidationError(ErrMsgWorkloadIdentityRequired)
	}

	for _, role := range spec.RoleBindings {
		if err := ValidateIAMRole(role); err != nil {
			return "", err
		}
	}

	labels := buildLabels(spec.Metadata)
	memberStr := fmt.Sprintf(WorkloadIdentityMember, spec.ProjectID, spec.Namespace, spec.KSAName)

	manifests := make([]GenericKCCManifest, 0, 1+len(spec.RoleBindings))

	wiBinding := GenericKCCManifest{
		APIVersion: ConfigConnectorAPIVersionIAM,
		Kind:       KindIAMPolicyMember,
		Metadata: KCCMetadata{
			Name:      fmt.Sprintf(PrefixWIBinding, spec.KSAName),
			Namespace: spec.Namespace,
			Labels:    labels,
		},
		Spec: map[string]any{
			KeyMember: memberStr,
			KeyRole:   WorkloadIdentityUserRole,
			KeyResourceRef: map[string]string{
				KeyAPIVersion: ConfigConnectorAPIVersionIAM,
				KeyKind:       KindIAMServiceAccount,
				KeyExternal:   spec.GSAEmail,
			},
		},
	}
	manifests = append(manifests, wiBinding)

	for _, role := range spec.RoleBindings {
		roleSlug := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(role, "roles/", ""), ".", Hyphen))
		roleManifest := GenericKCCManifest{
			APIVersion: ConfigConnectorAPIVersionIAM,
			Kind:       KindIAMPolicyMember,
			Metadata: KCCMetadata{
				Name:      fmt.Sprintf("%s-%s", spec.KSAName, roleSlug),
				Namespace: spec.Namespace,
				Labels:    labels,
			},
			Spec: map[string]any{
				KeyMember: fmt.Sprintf(MemberGSAPrefix, spec.GSAEmail),
				KeyRole:   role,
			},
		}
		manifests = append(manifests, roleManifest)
	}

	var sb strings.Builder
	for i, m := range manifests {
		data, err := yaml.Marshal(m)
		if err != nil {
			return "", fmt.Errorf(ErrMsgMarshalKCCFmt, err)
		}
		if i > 0 {
			sb.WriteString(YAMLDocSeparator)
		}
		sb.Write(data)
	}

	return sb.String(), nil
}

func GenerateDNSRecordManifest(spec DNSRecordSpec) (string, error) {
	if err := ValidateMetadata(spec.Metadata); err != nil {
		return "", err
	}
	if spec.ZoneName == "" || spec.RecordName == "" || spec.RecordType == "" {
		return "", errors.NewValidationError(ErrMsgDNSRequired)
	}
	if spec.TTL <= 0 {
		return "", errors.NewValidationError(ErrMsgDNSTTLInvalid)
	}

	rrdatas := spec.TargetIPs
	if spec.RecordType == DNSTypeCNAME {
		if spec.TargetHost == "" {
			return "", errors.NewValidationError(ErrMsgCNAMETargetRequired)
		}
		rrdatas = []string{spec.TargetHost}
	} else if len(rrdatas) == 0 {
		return "", errors.NewValidationError(fmt.Sprintf(ErrMsgTargetIPsRequired, spec.RecordType))
	}

	dnsName := spec.RecordName
	if !strings.HasSuffix(dnsName, DotSuffix) {
		dnsName += DotSuffix
	}

	labels := buildLabels(spec.Metadata)
	sanitizedName := strings.ReplaceAll(strings.TrimSuffix(spec.RecordName, DotSuffix), DotSuffix, Hyphen)

	manifest := GenericKCCManifest{
		APIVersion: ConfigConnectorAPIVersionDNS,
		Kind:       KindDNSRecordSet,
		Metadata: KCCMetadata{
			Name:   fmt.Sprintf(PrefixDNSName, sanitizedName),
			Labels: labels,
		},
		Spec: map[string]any{
			KeyName:    dnsName,
			KeyType:    spec.RecordType,
			KeyTTL:     spec.TTL,
			KeyRRDatas: rrdatas,
			KeyManagedZoneRef: map[string]string{
				KeyName: spec.ZoneName,
			},
		},
	}

	data, err := yaml.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf(ErrMsgMarshalDNSFmt, err)
	}
	return string(data), nil
}

func GenerateCloudSQLBindingManifest(spec CloudSQLBindingSpec) (string, error) {
	if err := ValidateMetadata(spec.Metadata); err != nil {
		return "", err
	}
	if spec.InstanceID == "" || spec.DatabaseName == "" || spec.AppUser == "" {
		return "", errors.NewValidationError(ErrMsgSQLRequired)
	}

	labels := buildLabels(spec.Metadata)

	dbManifest := GenericKCCManifest{
		APIVersion: ConfigConnectorAPIVersionSQL,
		Kind:       KindSQLDatabase,
		Metadata: KCCMetadata{
			Name:   fmt.Sprintf(PrefixSQLDB, spec.InstanceID, spec.DatabaseName),
			Labels: labels,
		},
		Spec: map[string]any{
			KeyName: spec.DatabaseName,
			KeyInstanceRef: map[string]string{
				KeyName: spec.InstanceID,
			},
		},
	}

	userSpec := map[string]any{
		KeyName: spec.AppUser,
		KeyInstanceRef: map[string]string{
			KeyName: spec.InstanceID,
		},
	}
	if spec.IAMAuthEnabled {
		userSpec[KeyType] = SQLUserTypeCloudIAM
		if spec.GSAEmail != "" {
			userSpec[KeyName] = strings.TrimSuffix(spec.GSAEmail, GSAEmailSuffix)
		}
	}

	userManifest := GenericKCCManifest{
		APIVersion: ConfigConnectorAPIVersionSQL,
		Kind:       KindSQLUser,
		Metadata: KCCMetadata{
			Name:   fmt.Sprintf(PrefixSQLUser, spec.InstanceID, spec.AppUser),
			Labels: labels,
		},
		Spec: userSpec,
	}

	var sb strings.Builder
	dbData, err := yaml.Marshal(dbManifest)
	if err != nil {
		return "", fmt.Errorf(ErrMsgMarshalSQLDBFmt, err)
	}
	sb.Write(dbData)
	sb.WriteString(YAMLDocSeparator)

	userData, err := yaml.Marshal(userManifest)
	if err != nil {
		return "", fmt.Errorf(ErrMsgMarshalSQLUserFmt, err)
	}
	sb.Write(userData)

	return sb.String(), nil
}
