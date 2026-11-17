package cloudresource

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"internal-developer-platform/src/shared/errors"
)

type GenericKCCManifest struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   KCCMetadata       `yaml:"metadata"`
	Spec       map[string]any    `yaml:"spec"`
}

type KCCMetadata struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace,omitempty"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

func ValidateMetadata(meta ResourceMetadata) error {
	if strings.TrimSpace(meta.RequestID) == "" {
		return errors.NewValidationError("request_id is mandatory for cloud resource")
	}
	if strings.TrimSpace(meta.Owner) == "" {
		return errors.NewValidationError("owner is mandatory for cloud resource")
	}
	if strings.TrimSpace(meta.CostCenter) == "" {
		return errors.NewValidationError("cost_center is mandatory for cloud resource")
	}
	if strings.TrimSpace(meta.WorkloadID) == "" {
		return errors.NewValidationError("workload_id is mandatory for cloud resource")
	}
	if strings.TrimSpace(meta.Environment) == "" {
		return errors.NewValidationError("environment is mandatory for cloud resource")
	}
	return nil
}

func ValidateIAMRole(role string) error {
	if ProhibitedIAMRoles[role] {
		return errors.NewForbiddenError(fmt.Sprintf("prohibited over-broad IAM role: %s", role))
	}
	if !PermittedIAMRoles[role] {
		return errors.NewForbiddenError(fmt.Sprintf("unauthorized IAM role not in platform allowlist: %s", role))
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
		return "", errors.NewValidationError("gsa_email, ksa_name, namespace, and project_id are required")
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
			Name:      fmt.Sprintf("%s-wi-binding", spec.KSAName),
			Namespace: spec.Namespace,
			Labels:    labels,
		},
		Spec: map[string]any{
			"member": memberStr,
			"role":   WorkloadIdentityUserRole,
			"resourceRef": map[string]string{
				"apiVersion": ConfigConnectorAPIVersionIAM,
				"kind":       KindIAMServiceAccount,
				"external":   spec.GSAEmail,
			},
		},
	}
	manifests = append(manifests, wiBinding)

	for _, role := range spec.RoleBindings {
		roleSlug := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(role, "roles/", ""), ".", "-"))
		roleManifest := GenericKCCManifest{
			APIVersion: ConfigConnectorAPIVersionIAM,
			Kind:       KindIAMPolicyMember,
			Metadata: KCCMetadata{
				Name:      fmt.Sprintf("%s-%s", spec.KSAName, roleSlug),
				Namespace: spec.Namespace,
				Labels:    labels,
			},
			Spec: map[string]any{
				"member": fmt.Sprintf("serviceAccount:%s", spec.GSAEmail),
				"role":   role,
			},
		}
		manifests = append(manifests, roleManifest)
	}

	var sb strings.Builder
	for i, m := range manifests {
		data, err := yaml.Marshal(m)
		if err != nil {
			return "", fmt.Errorf("failed to marshal KCC manifest: %w", err)
		}
		if i > 0 {
			sb.WriteString("---\n")
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
		return "", errors.NewValidationError("zone_name, record_name, and record_type are required")
	}
	if spec.TTL <= 0 {
		return "", errors.NewValidationError("ttl must be greater than zero")
	}

	rrdatas := spec.TargetIPs
	if spec.RecordType == DNSTypeCNAME {
		if spec.TargetHost == "" {
			return "", errors.NewValidationError("target_host is required for CNAME records")
		}
		rrdatas = []string{spec.TargetHost}
	} else if len(rrdatas) == 0 {
		return "", errors.NewValidationError("target_ips cannot be empty for record type " + spec.RecordType)
	}

	dnsName := spec.RecordName
	if !strings.HasSuffix(dnsName, ".") {
		dnsName += "."
	}

	labels := buildLabels(spec.Metadata)
	sanitizedName := strings.ReplaceAll(strings.TrimSuffix(spec.RecordName, "."), ".", "-")

	manifest := GenericKCCManifest{
		APIVersion: ConfigConnectorAPIVersionDNS,
		Kind:       KindDNSRecordSet,
		Metadata: KCCMetadata{
			Name:   fmt.Sprintf("dns-%s", sanitizedName),
			Labels: labels,
		},
		Spec: map[string]any{
			"name":    dnsName,
			"type":    spec.RecordType,
			"ttl":     spec.TTL,
			"rrdatas": rrdatas,
			"managedZoneRef": map[string]string{
				"name": spec.ZoneName,
			},
		},
	}

	data, err := yaml.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal DNS manifest: %w", err)
	}
	return string(data), nil
}

func GenerateCloudSQLBindingManifest(spec CloudSQLBindingSpec) (string, error) {
	if err := ValidateMetadata(spec.Metadata); err != nil {
		return "", err
	}
	if spec.InstanceID == "" || spec.DatabaseName == "" || spec.AppUser == "" {
		return "", errors.NewValidationError("instance_id, database_name, and app_user are required")
	}

	labels := buildLabels(spec.Metadata)

	dbManifest := GenericKCCManifest{
		APIVersion: ConfigConnectorAPIVersionSQL,
		Kind:       KindSQLDatabase,
		Metadata: KCCMetadata{
			Name:   fmt.Sprintf("db-%s-%s", spec.InstanceID, spec.DatabaseName),
			Labels: labels,
		},
		Spec: map[string]any{
			"name": spec.DatabaseName,
			"instanceRef": map[string]string{
				"name": spec.InstanceID,
			},
		},
	}

	userSpec := map[string]any{
		"name": spec.AppUser,
		"instanceRef": map[string]string{
			"name": spec.InstanceID,
		},
	}
	if spec.IAMAuthEnabled {
		userSpec["type"] = SQLUserTypeCloudIAM
		if spec.GSAEmail != "" {
			userSpec["name"] = strings.TrimSuffix(spec.GSAEmail, ".gserviceaccount.com")
		}
	}

	userManifest := GenericKCCManifest{
		APIVersion: ConfigConnectorAPIVersionSQL,
		Kind:       KindSQLUser,
		Metadata: KCCMetadata{
			Name:   fmt.Sprintf("user-%s-%s", spec.InstanceID, spec.AppUser),
			Labels: labels,
		},
		Spec: userSpec,
	}

	var sb strings.Builder
	dbData, err := yaml.Marshal(dbManifest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal SQLDatabase manifest: %w", err)
	}
	sb.Write(dbData)
	sb.WriteString("---\n")

	userData, err := yaml.Marshal(userManifest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal SQLUser manifest: %w", err)
	}
	sb.Write(userData)

	return sb.String(), nil
}
