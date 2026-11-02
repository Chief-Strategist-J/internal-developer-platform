package queries

const (
	FlowInsertServiceDescriptor = `
		INSERT INTO service_descriptors (id, tenant_id, name, owner, lifecycle, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
	`
	FlowSelectServiceDescriptorByID = `
		SELECT id, tenant_id, name, owner, lifecycle, status, version, created_at, updated_at
		FROM service_descriptors
		WHERE tenant_id = $1 AND id = $2;
	`
)
