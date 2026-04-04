---
title: Reports & Analytics
weight: 7
---

**Reports and Analytics** provide visibility into case metrics, team performance, and workflow efficiency. Aceryx offers built-in reports, natural language querying, and custom SQL-based reporting.

## Built-In Reports

Aceryx includes a suite of pre-built reports accessible from the **Reports** menu.

### Cases Summary

Aggregates case counts and statistics by week, case type, or status.

**Metrics:**

- Total cases created.
- Cases by status (Open, In Progress, Completed, Cancelled).
- Average case age (time from creation to completion or current state).
- Trend line showing case volume over time.

**Filters:**

- Date range (last week, last month, last quarter, custom).
- Case type.
- Status.

**Use cases:**

- Understand case volume trends.
- Identify slow-moving case types.
- Monitor backlog growth.

### SLA Compliance

Tracks adherence to Service Level Agreements on human tasks.

**Metrics:**

- Percentage of tasks completed on-time.
- Percentage of tasks in warning (approaching deadline).
- Percentage of tasks breached (exceeded deadline).
- Escalation rate.

**Filters:**

- Date range.
- Case type.
- Assigned user or role.

**Visualization:**

- Pie chart: on-time vs. warning vs. breached.
- Trend line: compliance improving or degrading over time.

**Use cases:**

- Monitor team SLA performance.
- Identify bottlenecks (case types with high breach rates).
- Evaluate staffing needs.

{{< callout type="info" >}}
SLA compliance is a key performance indicator. Regularly review this report to ensure your team meets customer commitments.
{{< /callout >}}

### Case Ageing

Shows how long cases remain in each workflow stage.

**Metrics:**

- Average duration per stage.
- Median duration per stage.
- 90th percentile duration (longest-running cases).

**Breakdown:**

- By case type.
- By stage (workflow step name).
- By assigned user or role.

**Visualization:**

- Bar chart: average time in each stage.
- Box-and-whisker: distribution of times (showing outliers).

**Use cases:**

- Identify bottleneck stages.
- Optimize slow stages (add resources, simplify process).
- Benchmark against historical performance.

### Cases by Stage

Snapshot of case distribution across workflow stages.

**Metrics:**

- Count of cases in each stage.
- Percentage breakdown.
- Cases stuck (no activity in 7+ days, configurable).

**Filters:**

- Date range (cases created or modified in range).
- Case type.

**Visualization:**

- Pie chart or stacked bar chart by stage.
- Highlight "stuck" cases in red.

**Use cases:**

- Monitor workflow distribution.
- Spot bottlenecks visually.
- Identify cases requiring manual intervention (stuck cases).

### Workload (Tasks per User)

Distributes task assignments across the team.

**Metrics:**

- Task count per user or role.
- Average SLA completion time per user.
- Tasks completed per user (completed count).
- Current open task count.

**Breakdown:**

- By user.
- By role.
- By case type.

**Visualization:**

- Bar chart: workload per user.
- Scatter plot: task volume vs. average completion time.

**Use cases:**

- Balance workload across team members.
- Identify overloaded or underutilized staff.
- Evaluate performance (task volume and SLA compliance).

### Decisions (Outcome Patterns)

Analyzes patterns in task outcomes and workflow decisions.

**Metrics:**

- Approval rate (percentage of tasks with "Approved" outcome).
- Rejection rate.
- Request for information rate (if this outcome is used).
- Outcome distribution across case types.

**Breakdown:**

- By case type.
- By assignee.
- By time period.

**Visualization:**

- Pie chart: outcome distribution.
- Trend line: approval rate changing over time (e.g., stricter approvals in recent weeks).

**Use cases:**

- Detect approval bias or inconsistency.
- Monitor decision quality.
- Identify case types with high rejection rates (may need process improvement).

## Natural Language Queries

Ask questions about your data in plain English. Aceryx uses an LLM to generate and execute SQL queries.

**API:**

```
POST /reports/ask
{
  "question": "How many loan applications were completed last week?"
}
```

**Process:**

1. The LLM reads your question.
2. It has access to the schema (cases table, steps table, audit log, etc.).
3. It generates a SQL query.
4. Aceryx executes the query.
5. The result is returned with a human-readable summary.

**Example questions:**

- "What is the average time for a loan application to reach approval?"
- "Which case type has the highest SLA breach rate?"
- "Show me cases created by Alice that are still in the approval stage."
- "How many tasks were escalated this month?"

**Limitations:**

- The LLM must generate valid SQL. Complex or ambiguous questions may produce incorrect queries.
- Results are limited to 10,000 rows (configurable).
- Queries run on a read replica if one is configured, to avoid impacting production.

{{< callout type="warning" >}}
Natural language queries are powerful but not perfect. Always verify unexpected results by reviewing the generated SQL or running custom queries.
{{< /callout >}}

## Custom Reports

Create and save custom reports using SQL.

**Creating a custom report:**

1. Click **New Report**.
2. Write a SQL query against the Aceryx schema (cases, case_data, steps, case_steps, tasks, audit_log, etc.).
3. Test the query.
4. Configure visualization type (table, bar, line, pie, number).
5. Save with a name and description.

**Available tables:**

- **cases**: Case metadata (id, case_number, case_type, status, created_at, updated_at, version, tenant_id).
- **case_data**: JSON case data (case_id, data, version).
- **case_steps**: Workflow step execution (case_id, step_id, type, status, activated_at, completed_at, outcome).
- **audit_log**: All events (id, case_id, action, actor, timestamp, details).

**Example query:**

```sql
SELECT
  c.case_number,
  c.case_type,
  COUNT(t.id) AS task_count,
  AVG(EXTRACT(EPOCH FROM (t.completed_at - t.activated_at)) / 3600) AS avg_hours_per_task
FROM cases c
LEFT JOIN steps s ON c.id = s.case_id
LEFT JOIN tasks t ON s.id = t.step_id
WHERE c.created_at >= NOW() - INTERVAL '30 days'
GROUP BY c.case_number, c.case_type
ORDER BY avg_hours_per_task DESC;
```

## Materialized Views

Aceryx pre-computes **materialized views** for fast aggregation, enabling efficient dashboards and reports. These are separate from standard summary views.

### Built-in Report Views

Three primary materialized views are provided for reporting:

- **mv_report_cases**: Summarizes case metrics by type, status, and date.
- **mv_report_steps**: Summarizes step execution metrics.
- **mv_report_tasks**: Summarizes task metrics including SLA compliance.

The **report_view_schemas** table describes the available columns in each view.

**Refreshed:** Periodically via background job.

**Usage:**

Report views support natural language querying via the API. You can also query them directly:

```sql
SELECT case_type, status, COUNT(*) as count
FROM mv_report_cases
WHERE created_date >= NOW() - INTERVAL '30 days'
GROUP BY case_type, status;
```

### Saved Reports

Custom reports are stored in the **saved_reports** table with the following structure:

- **original_question**: The natural language question (if generated via LLM).
- **query_sql**: The SQL query to execute.
- **visualisation**: The visualization type (table, bar, line, pie, number).
- **columns**: Explicit column selection.
- **parameters**: Query parameters for filtering.
- **is_published**: Whether the report is available to all users.
- **pinned**: Whether the report is pinned to the dashboard.
- **schedule**: Optional cron expression for scheduled execution.
- **recipients**: Email recipients for scheduled reports.

## Report Execution

### On Demand

Click a report in the Reports menu. The system:

1. Executes the query (or fetches from materialized view if available).
2. Renders the result as a table, chart, or both.
3. Provides export options (CSV, Excel, PDF).

### Scheduled

Create a **scheduled report** to run automatically and email results to stakeholders.

**Configuration:**

```
Report: SLA Compliance
Schedule: Every Monday at 9 AM
Recipients: manager@example.com, ops-team@example.com
Format: PDF with embedded charts
```

**Process:**

1. At the scheduled time, Aceryx executes the report query.
2. Renders the result as PDF (with charts).
3. Sends via email to configured recipients.

**Retention:**

Executed reports are stored for 30 days (configurable) and can be viewed in the Report History.

## Dashboard

The **Dashboard** (main page) provides an at-a-glance overview of key metrics.

**Default widgets:**

- **Cases by status**: Pie chart of Open, In Progress, Completed, Cancelled.
- **Recent cases**: List of 5 most recently created/modified cases.
- **SLA summary**: Counts of on-track, warning, and breached tasks.
- **Tasks assigned to you**: Quick access to your inbox.
- **Weekly case volume**: Trend line of cases created per week.
- **Team workload**: Bar chart of tasks per team member.

**Customization:**

Users can customize the dashboard by:

1. Clicking **Edit Dashboard**.
2. Adding/removing widgets.
3. Resizing and reordering.
4. Setting filters (e.g., show only loan cases, show only my team).

**Refresh:**

The dashboard updates every 30 seconds by default (configurable). Use the **Refresh** button for immediate update.

## Report Performance

For large datasets, reports may be slow. Optimize by:

1. **Using materialized views** when possible (pre-aggregated data).
2. **Adding date filters** to limit the dataset (e.g., last 30 days instead of all-time).
3. **Running custom reports on a read replica** (if configured).
4. **Scheduling heavy reports** during off-peak hours.

**Monitoring:**

- Dashboard shows query execution time.
- Slow queries (>5 seconds) trigger warnings.
- Logs record all report executions for audit and debugging.

## Export and Integration

**Export formats:**

- **CSV**: Comma-separated values (spreadsheet-compatible).
- **Excel**: XLSX format with formatting and multiple sheets.
- **PDF**: Formatted report with charts and styling.
- **JSON**: Raw data for programmatic consumption.

**API:**

```
GET /reports/{report_id}/execute?format=csv&start_date=2026-03-01&end_date=2026-03-31
```

**Email delivery:**

Configure scheduled reports to email recipients automatically. Reports are attached as files (CSV, Excel, or PDF).

**Webhooks:**

Send report results to external systems (data warehouse, BI tools, Slack) via outbound webhooks on completion.
