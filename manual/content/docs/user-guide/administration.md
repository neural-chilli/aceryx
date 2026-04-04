---
title: Administration
weight: 8
---

Aceryx administration covers user management, roles and permissions, branding, audit, data compliance, and backup/restore operations.

## User Management

**Principals** in Aceryx come in two types: **"human"** (for users) and **"agent"** (for automated identities). Administrators can create, modify, and deactivate principals.

**Access:** Administration → Users (requires `admin:users` permission).

### Creating a User

1. Click **New User**.
2. Enter:
   - **Name**: Full name.
   - **Email**: Unique email address.
   - **Username**: Unique identifier for API/CLI (optional).
3. Assign roles (covered below).
4. Click **Create**.

Aceryx sends a welcome email with a secure link for the user to set their password.

### Disabling Accounts

Disable a user without deleting their history:

1. Click the user in the Users list.
2. Click **Disable Account**.
3. The user cannot log in, but their audit trail and case associations remain.

Disabled users can be re-enabled at any time.

### Agents

Agents are automated principals used for system-to-system operations (e.g., inbound webhooks, scheduled bulk operations).

**Creating an agent:**

1. Click **New Agent**.
2. Enter name and description.
3. Generate an API key (shown once; copy and save securely).
4. Assign roles with appropriate permissions.

Agents authenticate via API key in the `Authorization: Bearer` header.

## Roles and Permissions

**Roles** are groups of permissions that can be assigned to users or agents. Create role-based access control to enforce the principle of least privilege.

**Access:** Administration → Roles (requires `admin:roles` permission).

### Creating a Role

1. Click **New Role**.
2. Enter role name (e.g., "Loan Officer", "Approver", "Admin").
3. Select permissions from the full list (see below).
4. Click **Create**.

### Assigning Roles to Users

1. Open a user.
2. Click **Edit Roles**.
3. Select one or more roles to assign.
4. Click **Save**.

Users inherit all permissions from all assigned roles.

### Permission List

Permissions support wildcards (e.g., `*`, `admin:*`, `workflows:*`).

**Full permission list:**

- **cases:create**: Create new cases.
- **cases:read**: View case details, case data, and audit log.
- **cases:update**: Modify case data and metadata.
- **cases:delete**: Delete cases.
- **tasks:read**: View task details.
- **tasks:claim**: Claim role-assigned tasks.
- **tasks:complete**: Complete assigned tasks.
- **reports:read**: Run built-in and custom reports.
- **admin:users**: Create, modify, disable users and agents.
- **admin:roles**: Create, modify roles and assign permissions.
- **admin:tenant**: Manage tenant branding, themes, terminology (company-wide settings).
- **audit:read**: View and export audit log, request data erasure.
- **vault:read**: View vault documents.
- **vault:upload**: Upload documents to cases.
- **vault:delete**: Delete documents (with audit trail).

### Default Roles

Aceryx provides pre-built roles auto-seeded per tenant:

- **Admin** (`*`): Full access to all permissions.
- **Workflow Designer** (`workflows:*`, `cases:read`): Create, edit, and publish workflows; read cases.
- **Case Worker** (`cases:read`, `cases:update`, `tasks:claim`, `tasks:complete`, `vault:download`, `vault:upload`): Work on cases and tasks.
- **Viewer** (`cases:read`, `vault:download`): Read-only access to cases and documents.

## Tenant Branding

Customize Aceryx to reflect your organization's brand.

**Access:** Administration → Tenant (requires `admin:tenant` permission).

### Branding Configuration

Branding settings are stored as JSONB in the `tenants.branding` column and include:

- **company_name**: Displayed in header and emails.
- **logo_url**: URL to a logo image (PNG, SVG, or JPEG). Displayed in the header and login page.
- **favicon_url**: URL to a favicon for browser tabs.
- **colors**: Color configuration:
  - **primary**: Main accent color used in buttons, links, and headers.
  - **secondary**: Accent color for secondary elements.
  - **accent**: Additional accent color for highlights.
- **powered_by**: Optional text/branding to display in the footer (e.g., "Powered by Aceryx").

## Themes

Create and manage **themes** for different light/dark modes and colour schemes.

**Access:** Administration → Themes (requires `admin:tenant` permission).

### Default Themes

Four default themes are auto-seeded per tenant:

- **Light**: Clean white background with dark text.
- **Dark**: Dark background with light text, easier on eyes in low-light environments.
- **High Contrast Light**: Enhanced contrast for accessibility.
- **High Contrast Dark**: Enhanced contrast for accessibility in dark mode.

These themes are automatically created via a database trigger when a new tenant is created.

### Custom Themes

1. Click **New Theme**.
2. Name the theme (e.g., "Custom Brand Theme").
3. Configure colours:
   - Background, text, links, buttons.
   - Hover states, disabled states.
4. Set the default theme in **Settings**.

Users can override the default theme in their personal **Preferences** (account settings).

## Terminology Customization

Rename default terms to match your organization's language.

**Access:** Administration → Terminology (requires `admin:tenant` permission).

### Terminology Configuration

Terminology is stored as JSONB in the `tenants.terminology` column and includes:

- **case**: Singular form (e.g., "Application").
- **cases**: Plural form (e.g., "Applications").
- **task**: Singular form (e.g., "Action").
- **tasks**: Plural form (e.g., "Actions").
- **inbox**: The inbox label (default: "Inbox").

When you change a term, the UI updates globally. All documentation and help text within the app reflects the new terminology.

{{< callout type="info" >}}
Use terminology that matches your industry and organization. Familiarity reduces onboarding time and increases user adoption.
{{< /callout >}}

{{< callout type="info" >}}
Use terminology that matches your industry and organization. Familiarity reduces onboarding time and increases user adoption.
{{< /callout >}}

## Audit Log

The **audit log** records every action in the system for compliance, debugging, and security purposes. Both general audit events and authentication events are tracked.

**Access:** Administration → Audit Log (requires `admin:audit` permission) via **GET /audit** endpoint. Authentication events are tracked separately in the `auth_events` table.

### Viewing the Audit Log

1. Click **Audit Log**.
2. Filter by:
   - **Date range**: When the action occurred.
   - **Actor**: Who performed the action (user, agent, system).
   - **Action**: What happened (create case, update task, delete document, etc.).
   - **Resource**: Which case, task, user, etc. was affected.
3. Click a row to see full details (JSON).

### Events Logged

- **Cases**: Created, updated, closed, cancelled.
- **Tasks**: Assigned, claimed, completed, reassigned, escalated.
- **Workflows**: Published, withdrawn, edited.
- **Documents**: Uploaded, downloaded, deleted, signed URL generated.
- **Users**: Created, disabled, role assigned.
- **Audit log**: Exported, hash chain verified.
- **Configuration**: Tenant branding, themes, terminology changed.

### Hash Chain Integrity

Aceryx implements a **hash chain** to detect tampering with the audit log.

Each audit entry contains:

- `hash`: SHA-256 of (previous_hash + current_entry).
- `previous_hash`: Hash of the prior entry.

If any entry is modified, its hash changes, breaking the chain. This enables detection of tampering.

**Verify integrity:**

```
GET /audit/verify-chain
```

Returns:

```json
{
  "status": "valid",
  "entries_checked": 10000,
  "first_entry": "2026-01-01T00:00:00Z",
  "last_entry": "2026-04-04T14:30:00Z"
}
```

### Exporting the Audit Log

Export to CSV or JSON for external storage or compliance purposes.

**API:**

```
GET /audit/export?format=csv&start_date=2026-03-01&end_date=2026-03-31
```

**Via UI:**

1. Click **Audit Log**.
2. Optionally filter to a date range.
3. Click **Export as CSV** or **Export as JSON**.
4. File downloads.

The exported file is read-only and suitable for archival or compliance submission.

## Data Erasure (GDPR)

Aceryx supports GDPR-compliant data erasure for personal data.

**Access:** Administration → Data Erasure (requires `admin:audit` permission).

### Initiating an Erasure Request

**Scenario:** A customer requests deletion of their personal data under GDPR.

1. Click **New Erasure Request**.
2. Specify the **data subject** (e.g., customer name or ID).
3. Optionally select specific cases or all cases for the subject.
4. Click **Request Erasure**.

### Erasure Process

1. All documents attached to selected cases are **securely deleted** (overwritten with random data).
2. Case data is **redacted**: personal fields (name, email, etc.) are removed or anonymised.
3. The audit log records the erasure request, timestamp, and reason.
4. A compliance certificate is generated, suitable for proof of erasure.

### Verification

Export an erasure report to confirm compliance:

- Cases erased.
- Erasure date and reason.
- Verification hash (proving deletion occurred).
- Compliance certificate for regulatory submission.

{{< callout type="warning" >}}
Data erasure is permanent and irreversible. Ensure you have legal authority and proper documentation before initiating erasure.
{{< /callout >}}

## Backup and Restore

Aceryx provides CLI commands for backup and restore operations.

**Access:** Root/administrator access to the Aceryx server.

### Backup Command

Create a full backup of the database, vault, and metadata:

```bash
./aceryx backup --output /backups/aceryx-2026-04-04.tar.gz
```

**Backup contents:**

- **Database dump**: PostgreSQL dump (DDL, DML, sequences).
- **Vault archive**: All documents (content-addressed files).
- **Metadata**: Encryption keys, configuration, version info.
- **Manifest**: JSON file listing all backed-up components.

**Options:**

- `--output`: Output file path (required).
- `--compress`: Compression format (gzip, bzip2, xz; default: gzip).
- `--verify`: Verify backup integrity after creation.

**Duration:** Depends on database size and vault content. For a 100 GB vault, expect 10-30 minutes.

### Restore Command

Restore from a backup:

```bash
./aceryx restore --input /backups/aceryx-2026-04-04.tar.gz --target /tmp/restore
```

**Options:**

- `--input`: Backup file path (required).
- `--target`: Restore directory (required; must be empty).
- `--verify`: Verify restored data integrity.
- `--dry-run`: Simulate restore without writing data.

**Process:**

1. Extracts the tar.gz file.
2. Validates the manifest.
3. Restores the database dump.
4. Restores vault files.
5. Verifies data integrity (if `--verify` is set).

**Output:**

```
Restore completed successfully
Database: aceryx_restored
Vault: /tmp/restore/vault
Manifest: /tmp/restore/manifest.json
Start time: 2026-04-04T14:30:00Z
End time: 2026-04-04T14:35:00Z
Duration: 5m 30s
```

### Verification Command

Verify a backup's integrity without restoring:

```bash
./aceryx backup verify /backups/aceryx-2026-04-04.tar.gz
```

**Checks:**

- Archive structure is valid.
- All files are readable and uncorrupted.
- Manifest checksums match.
- Database dump is syntactically valid SQL.
- Encryption keys are present.

**Output:**

```
Backup verification successful
Archive: aceryx-2026-04-04.tar.gz
Size: 523 MB
Files: 4250
Database dump: valid
Vault files: 3847 files, 512 GB total
Encryption key present: yes
Created: 2026-04-04T12:00:00Z
Expires: 2026-05-04T12:00:00Z (configurable retention)
```

### Backup Strategy

**Recommended approach:**

1. **Daily incremental backups**: Use `aceryx backup` with `--incremental` flag to create smaller daily backups.
2. **Weekly full backups**: Create full backups each week for complete restore capability.
3. **Off-site storage**: Copy backups to cloud storage (S3, GCS, Azure Blob) for disaster recovery.
4. **Retention policy**: Keep daily backups for 7 days, weekly for 4 weeks, monthly for 1 year.
5. **Verification**: Run `backup verify` on all backups weekly to detect corruption early.

**Example backup script:**

```bash
#!/bin/bash
BACKUP_DATE=$(date +%Y-%m-%d)
./aceryx backup --output /backups/daily-${BACKUP_DATE}.tar.gz --compress gzip --verify
aws s3 cp /backups/daily-${BACKUP_DATE}.tar.gz s3://my-backups/aceryx/
# Keep only 7 days of local backups
find /backups -name "daily-*.tar.gz" -mtime +7 -delete
```

### Disaster Recovery

**In case of data loss or corruption:**

1. Stop Aceryx: `systemctl stop aceryx`.
2. Restore from the most recent backup: `./aceryx restore --input /backups/aceryx-latest.tar.gz --target /restore`.
3. Verify the restored database: `./aceryx backup verify /restore/manifest.json`.
4. Migrate restored data to production (depends on your setup—consult deployment documentation).
5. Restart Aceryx: `systemctl start aceryx`.
6. Verify functionality and data integrity.

**Recovery Time Objective (RTO):** 30 minutes to 2 hours, depending on data size.

**Recovery Point Objective (RPO):** Depends on backup frequency (e.g., 24 hours for daily backups).
