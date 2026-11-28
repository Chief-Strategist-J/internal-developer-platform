package adapters

import (
	"context"
	"database/sql"
	"sync"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/gitops-reconciliation/internal/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/domain/rules"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/errors"
)

type secretMechanismHandler func(ctx context.Context, cfg contracts.SecretBackendConfig, ref contracts.SecretReference) (map[string]string, error)

type ConfigurableSecretAdapter struct {
	mu       sync.RWMutex
	db       *sql.DB
	configs  map[string]contracts.SecretBackendConfig
	handlers map[contracts.SecretMechanismType]secretMechanismHandler
}

func NewConfigurableSecretAdapter() *ConfigurableSecretAdapter {
	return NewConfigurableSecretAdapterWithDB(nil)
}

func NewConfigurableSecretAdapterWithDB(db *sql.DB) *ConfigurableSecretAdapter {
	return &ConfigurableSecretAdapter{
		db:      db,
		configs: make(map[string]contracts.SecretBackendConfig),
		handlers: map[contracts.SecretMechanismType]secretMechanismHandler{
			contracts.SecretMechanismSealedSecrets:           resolveSealedSecretRef,
			contracts.SecretMechanismExternalSecretsOperator: resolveExternalSecretRef,
			contracts.SecretMechanismSecretsStoreCSI:         resolveCSISecretRef,
		},
	}
}

func (a *ConfigurableSecretAdapter) ConfigureSecretMechanism(
	ctx context.Context,
	cfg contracts.SecretBackendConfig,
) (*contracts.SecretBackendConfig, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "gitops.secret_adapter.configure",
		attribute.String("tenant.id", cfg.TenantID),
		attribute.String("secret.mechanism", string(cfg.Mechanism)),
	)
	defer span.End()

	if err := rules.ValidateSecretMechanismConfig(cfg); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	if a.db != nil {
		if _, err := a.db.ExecContext(ctx, queries.FLOW_UPSERT_SECRET_MECHANISM_CONFIG,
			cfg.TenantID, string(cfg.Mechanism), cfg.VaultEndpoint, cfg.ClusterScope,
			cfg.SecretStoreRef, cfg.CSIDriverName, cfg.EncryptionKeyRef, cfg.Enabled,
		); err != nil {
			observability.RecordError(span, err)
			return nil, errors.NewInternalError(constants.ErrSecretPersistFailed)
		}
	}

	a.mu.Lock()
	a.configs[cfg.TenantID] = cfg
	a.mu.Unlock()

	saved := cfg
	return &saved, nil
}

func (a *ConfigurableSecretAdapter) GetSecretMechanismConfig(
	ctx context.Context,
	tenantID string,
) (*contracts.SecretBackendConfig, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "gitops.secret_adapter.get_config",
		attribute.String("tenant.id", tenantID),
	)
	defer span.End()

	if a.db != nil {
		var cfg contracts.SecretBackendConfig
		var mech string
		err := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_SECRET_MECHANISM_CONFIG, tenantID).Scan(
			&cfg.TenantID, &mech, &cfg.VaultEndpoint, &cfg.ClusterScope,
			&cfg.SecretStoreRef, &cfg.CSIDriverName, &cfg.EncryptionKeyRef, &cfg.Enabled,
		)
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError(constants.ErrSecretConfigNotFound)
		}
		if err != nil {
			observability.RecordError(span, err)
			return nil, errors.NewInternalError(constants.ErrSecretLoadFailed)
		}
		cfg.Mechanism = contracts.SecretMechanismType(mech)
		return &cfg, nil
	}

	a.mu.RLock()
	cfg, ok := a.configs[tenantID]
	a.mu.RUnlock()
	if !ok {
		return nil, errors.NewNotFoundError(constants.ErrSecretConfigNotFound)
	}
	copyCfg := cfg
	return &copyCfg, nil
}

func (a *ConfigurableSecretAdapter) ResolveSecretReference(
	ctx context.Context,
	tenantID string,
	ref contracts.SecretReference,
) (map[string]string, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "gitops.secret_adapter.resolve",
		attribute.String("tenant.id", tenantID),
		attribute.String("secret.name", ref.Name),
	)
	defer span.End()

	a.mu.RLock()
	cfg, ok := a.configs[tenantID]
	a.mu.RUnlock()
	if !ok {
		err := errors.NewNotFoundError(constants.ErrSecretConfigNotFound)
		observability.RecordError(span, err)
		return nil, err
	}

	handler, exists := a.handlers[cfg.Mechanism]
	if !exists {
		err := errors.NewValidationError(constants.ErrSecretUnsupportedConfigured)
		observability.RecordError(span, err)
		return nil, err
	}

	return handler(ctx, cfg, ref)
}

func resolveSealedSecretRef(
	_ context.Context,
	cfg contracts.SecretBackendConfig,
	ref contracts.SecretReference,
) (map[string]string, error) {
	if len(ref.EncryptedData) == 0 {
		return nil, errors.NewValidationError(constants.ErrSecretEncryptedDataRequired)
	}
	return map[string]string{
		constants.FieldKeyKind:             constants.KindSealedSecret,
		constants.FieldKeyName:             ref.Name,
		constants.FieldKeyNamespace:        ref.Namespace,
		constants.FieldKeyEncryptionKeyRef: cfg.EncryptionKeyRef,
	}, nil
}

func resolveExternalSecretRef(
	_ context.Context,
	cfg contracts.SecretBackendConfig,
	ref contracts.SecretReference,
) (map[string]string, error) {
	if ref.RemoteKeyRef == "" {
		return nil, errors.NewValidationError(constants.ErrSecretRemoteKeyRefRequired)
	}
	return map[string]string{
		constants.FieldKeyKind:           constants.KindExternalSecret,
		constants.FieldKeyName:           ref.Name,
		constants.FieldKeyNamespace:      ref.Namespace,
		constants.FieldKeySecretStoreRef: cfg.SecretStoreRef,
		constants.FieldKeyRemoteKeyRef:   ref.RemoteKeyRef,
	}, nil
}

func resolveCSISecretRef(
	_ context.Context,
	cfg contracts.SecretBackendConfig,
	ref contracts.SecretReference,
) (map[string]string, error) {
	if ref.RemoteKeyRef == "" {
		return nil, errors.NewValidationError(constants.ErrSecretRemoteKeyRefRequired)
	}
	return map[string]string{
		constants.FieldKeyKind:          constants.KindSecretProviderClass,
		constants.FieldKeyName:          ref.Name,
		constants.FieldKeyNamespace:     ref.Namespace,
		constants.FieldKeyCSIDriverName: cfg.CSIDriverName,
		constants.FieldKeyRemoteKeyRef:  ref.RemoteKeyRef,
	}, nil
}
