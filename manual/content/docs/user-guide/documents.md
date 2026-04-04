---
title: Documents
weight: 6
---

The **Vault** is Aceryx's secure, content-addressed document storage system. It enables cases to attach files, manage access, and ensure compliance with data protection regulations.

## The Vault Architecture

The Vault uses **content-hash addressing**: files are stored and retrieved using their SHA-256 hash rather than traditional file names.

**Benefits:**

- **Automatic deduplication**: If two identical files are uploaded, only one copy is stored. Multiple cases can reference the same content hash, saving storage.
- **Integrity verification**: The hash serves as a cryptographic fingerprint. Any modification to the file changes the hash.
- **Deterministic**: The same file always produces the same hash, enabling reliable comparisons.

## Uploading Documents

### Via User Interface

1. Open a case or a task in a case.
2. Click **Upload Document** or **Attach File**.
3. Select a file from your computer.
4. Optionally provide metadata: filename, description, tags.
5. The file is uploaded, hashed, and stored in the Vault.

### Via API

Upload a document to a case:

```
POST /cases/{case_id}/documents
Content-Type: multipart/form-data

file=@/path/to/document.pdf
filename=Loan_Application.pdf
```

The API returns the document's content hash, metadata, and a download URL.

### Via Connector

The Document Generation connector can programmatically create documents (e.g., PDF approval letters) and attach them to cases.

## Content-Hash Addressing

Every document is indexed by its **content hash** (SHA-256):

```
document_id = SHA256(file_contents)
```

**Storage path:**

```
{ACERYX_VAULT_PATH}/{first_2_chars}/{next_2_chars}/{rest_of_hash}
```

Example: `SHA256(my_file.pdf) = abc123def456...` is stored at `/vault/ab/c1/23def456...`

**Deduplication:**

If a user uploads the same file (bit-for-bit identical), the system:

1. Computes its hash.
2. Checks if a file with that hash already exists in the Vault.
3. If yes, creates a metadata entry pointing to the existing file (no new storage used).

This is particularly valuable for standardized forms, templates, or supporting documents that are uploaded repeatedly.

## Downloading Documents

### Via User Interface

1. Open a case or task.
2. Click the **Documents** section.
3. Click the download icon next to a document.
4. The file downloads to your computer.

**Access Control:**

Downloads are subject to RBAC checks. You must have the `vault:download` permission to download documents.

### Via API

```
GET /cases/{case_id}/documents/{document_hash}
```

Returns the file with appropriate HTTP headers (`Content-Disposition: attachment`).

## Signed URLs

Generate **signed, time-limited URLs** for sharing documents with external parties without granting them access to Aceryx.

**API:**

```
POST /cases/{case_id}/documents/{document_hash}/signed-url
{
  "expires_in_hours": 24
}
```

**Response:**

```json
{
  "url": "https://aceryx.example.com/vault/signed/{doc_id}?token=...",
  "expires_at": "2026-04-05T14:30:00Z"
}
```

Signed URLs are generated using **HMAC-SHA256** with the `ACERYX_VAULT_SIGNING_KEY` environment variable. If `ACERYX_VAULT_SIGNING_KEY` is not set, the system falls back to using `ACERYX_JWT_SECRET`.

The recipient can download the document using the signed URL without authentication. The URL expires after the specified period.

{{< callout type="info" >}}
Signed URLs are ideal for sharing documents with customers, vendors, or partners who don't have Aceryx accounts.
{{< /callout >}}

**Security Considerations:**

- Signed URLs are cryptographically signed with HMAC-SHA256 and tamper-proof.
- They are time-limited; set an appropriate expiration based on your security policy.
- Document access is logged in the audit trail, including signed URL usage.

## Document Metadata

Each document record stores:

- **Content hash**: SHA-256 of the file.
- **Filename**: Original filename provided by the uploader.
- **File size**: Size in bytes.
- **MIME type**: Content type (e.g., `application/pdf`, `image/png`).
- **Uploaded at**: Timestamp of upload.
- **Uploaded by**: User who uploaded the document.
- **Description/tags**: Optional user-provided metadata.
- **Extracted text**: OCR output for searchable text content.
- **Extracted data**: Entities and summaries extracted from documents.
- **Embedding**: Vector embedding (pgvector) for semantic search.

This metadata is indexed and searchable via full-text search across cases. Documents are stored with a URI format of `local://{tenantID}/{contentHash}.{ext}`.

## Document Lifecycle and Cleanup

### Background Cleanup Job

A background job (configurable via `ACERYX_VAULT_CLEANUP_INTERVAL`, default 24 hours) removes **orphaned documents**—files in the Vault that are no longer referenced by any case.

**When does a document become orphaned?**

- Its case is deleted.
- The document reference is manually removed from a case.
- A case is soft-deleted (marked for erasure).

**Cleanup process:**

1. Query all active cases and collect referenced document hashes.
2. Compare against files in the Vault.
3. Remove unreferenced files.
4. Log the cleanup operation.

This automatic cleanup prevents storage bloat and reduces costs, especially in systems with high document churn.

{{< callout type="info" >}}
The cleanup job respects document retention policies configured per case type. Some case types may require documents to be retained even after the case completes.
{{< /callout >}}

## Data Erasure and GDPR Compliance

Aceryx supports **GDPR-compliant data erasure** for documents and case data.

### Soft Delete with Erasure Timestamp

When a document is marked for erasure (due to a GDPR data subject request):

1. The document record is marked with an `erasure_requested_at` timestamp.
2. The physical file is overwritten with random data or securely deleted.
3. The metadata is retained for audit purposes (showing what was deleted and when) but the content is gone.

### Erasure API

```
DELETE /cases/{case_id}/documents/{document_hash}?reason=gdpr_right_to_be_forgotten
```

The deletion is recorded in the audit log with the reason and timestamp.

### Data Subject Requests

When a customer requests erasure under GDPR:

1. Admin initiates an erasure request for the customer's cases.
2. All documents attached to those cases are marked for erasure.
3. Case data is redacted.
4. The audit trail shows what was erased and when.

### Compliance Audit

Export an audit report showing:

- Erasure requests processed.
- Dates and reasons for erasure.
- Verification hash (proving deletion occurred).

## Storage Backends

### Local Filesystem (Default)

Documents are stored on the server's local filesystem.

**Configuration:**

```bash
export ACERYX_VAULT_PATH=/var/lib/aceryx/vault
```

**Setup:**

```bash
mkdir -p /var/lib/aceryx/vault
chmod 700 /var/lib/aceryx/vault
```

**Advantages:**

- Simple, no external dependencies.
- Low latency.
- No per-file cost.

**Disadvantages:**

- Single server (no redundancy without NFS or similar).
- Limited to disk capacity of the server.
- Backups require filesystem-level tools.

### Cloud Storage (Enterprise Edition)

Aceryx Enterprise Edition supports cloud backends:

- **Amazon S3**: Store documents in S3 buckets with encryption.
- **Google Cloud Storage**: GCS buckets with IAM access control.

**Configuration (S3 example):**

```bash
export ACERYX_VAULT_BACKEND=s3
export ACERYX_VAULT_S3_BUCKET=my-org-vault
export ACERYX_VAULT_S3_REGION=us-east-1
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
```

**Advantages:**

- Unlimited scalability.
- Built-in redundancy and availability.
- Encryption at rest and in transit.
- Compliance features (e.g., versioning, lifecycle policies).
- Pay-as-you-go pricing.

**Configuration (GCS example):**

```bash
export ACERYX_VAULT_BACKEND=gcs
export ACERYX_VAULT_GCS_BUCKET=my-org-vault
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json
```

## Accessing Documents from Workflows

Connectors and agents can access documents as part of case context.

### For Agents

When assembling context for an LLM, the system can:

- **Embed document text**: Include small documents or summaries directly in the prompt.
- **Vector search**: For large document sets, the agent performs semantic search to retrieve relevant excerpts.

Example agent prompt:

```
Analyze the attached contracts:

Contract 1: {{document_text.hash_abc123}}

{{step_results.doc_search.results}}

Summarize the key terms and highlight any risks.
```

### For Connectors

Connectors can:

- **Attach documents to emails**: The Email connector can include vault documents as attachments.
- **Include document links**: Pass signed URLs in HTTP or Slack payloads.
- **Reference documents in Jira**: Link documents when creating Jira issues.

## Monitoring and Metrics

**Vault metrics (Prometheus):**

- `aceryx_vault_documents_total`: Total documents by status (active, orphaned, erased).
- `aceryx_vault_storage_bytes`: Storage used by backend.
- `aceryx_vault_downloads_total`: Document downloads by document type.
- `aceryx_vault_deduplication_ratio`: Percentage of storage saved by deduplication.

**Audit log:**

Every document action is logged:

- Upload, download, delete.
- Signed URL generation and usage.
- Erasure requests and completion.
- User and timestamp for each action.

**Health checks:**

Run `./aceryx health check vault` to verify:

- Vault path is accessible and writable.
- Cloud backend credentials are valid.
- No orphaned documents exceeding retention policies.
