CREATE TABLE IF NOT EXISTS auth_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID REFERENCES tenants(id),
    principal_id  UUID REFERENCES principals(id),
    event_type    TEXT NOT NULL,
    success       BOOLEAN NOT NULL DEFAULT false,
    permission    TEXT,
    resource_path TEXT,
    ip_address    TEXT,
    user_agent    TEXT,
    data          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN auth_events.data IS
'{"reason":"string","email":"string"}';

CREATE INDEX IF NOT EXISTS idx_auth_events_tenant_created ON auth_events(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_auth_events_principal_created ON auth_events(principal_id, created_at DESC) WHERE principal_id IS NOT NULL;

CREATE OR REPLACE FUNCTION seed_default_roles_for_tenant(p_tenant_id UUID)
RETURNS VOID
LANGUAGE plpgsql
AS $$
DECLARE
    admin_role_id UUID;
    workflow_role_id UUID;
    worker_role_id UUID;
    viewer_role_id UUID;
BEGIN
    INSERT INTO roles (tenant_id, name, description)
    VALUES
        (p_tenant_id, 'admin', 'Full access to all tenant resources'),
        (p_tenant_id, 'workflow_designer', 'Design and deploy workflows'),
        (p_tenant_id, 'case_worker', 'Work assigned cases and tasks'),
        (p_tenant_id, 'viewer', 'Read-only case access')
    ON CONFLICT (tenant_id, name) DO NOTHING;

    SELECT id INTO admin_role_id FROM roles WHERE tenant_id = p_tenant_id AND name = 'admin';
    SELECT id INTO workflow_role_id FROM roles WHERE tenant_id = p_tenant_id AND name = 'workflow_designer';
    SELECT id INTO worker_role_id FROM roles WHERE tenant_id = p_tenant_id AND name = 'case_worker';
    SELECT id INTO viewer_role_id FROM roles WHERE tenant_id = p_tenant_id AND name = 'viewer';

    INSERT INTO role_permissions (role_id, permission)
    VALUES
        (admin_role_id, '*'),
        (admin_role_id, 'admin:tenant'),
        (workflow_role_id, 'workflows:*'),
        (workflow_role_id, 'cases:read'),
        (worker_role_id, 'cases:read'),
        (worker_role_id, 'cases:update'),
        (worker_role_id, 'tasks:claim'),
        (worker_role_id, 'tasks:complete'),
        (worker_role_id, 'vault:download'),
        (worker_role_id, 'vault:upload'),
        (viewer_role_id, 'cases:read'),
        (viewer_role_id, 'vault:download')
    ON CONFLICT DO NOTHING;
END;
$$;

CREATE OR REPLACE FUNCTION tenants_after_insert_seed_roles()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM seed_default_roles_for_tenant(NEW.id);
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_tenants_seed_roles ON tenants;

CREATE TRIGGER trg_tenants_seed_roles
AFTER INSERT ON tenants
FOR EACH ROW
EXECUTE FUNCTION tenants_after_insert_seed_roles();

DO $$
BEGIN
    PERFORM seed_default_roles_for_tenant(id)
    FROM tenants;
END $$;
