package queries

const (
	FlowInsertService = "INSERT INTO service_catalog (id, tenant_id, name, owner, lifecycle, status, version, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)"
	FlowSelectServiceByID = "SELECT id, tenant_id, name, owner, lifecycle, status, version, created_at, updated_at FROM service_catalog WHERE tenant_id = $1 AND id = $2"
)
