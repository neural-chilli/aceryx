package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/mcpserver"
)

type fakeCaseStore struct{}

func (fakeCaseStore) CreateCaseMCP(context.Context, uuid.UUID, uuid.UUID, string, map[string]any, bool) (uuid.UUID, string, error) {
	return uuid.MustParse("11111111-1111-1111-1111-111111111111"), "open", nil
}
func (fakeCaseStore) GetCaseMCP(context.Context, uuid.UUID, uuid.UUID) (mcpserver.CaseView, error) {
	return mcpserver.CaseView{}, nil
}
func (fakeCaseStore) UpdateCaseMCP(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, map[string]any) error {
	return nil
}
func (fakeCaseStore) SearchCasesMCP(context.Context, uuid.UUID, mcpserver.CaseSearchInput) ([]mcpserver.CaseSearchResult, int, error) {
	return nil, 0, nil
}

type fakeTaskStore struct{}

func (fakeTaskStore) ListTasksMCP(context.Context, uuid.UUID, mcpserver.TaskListInput) ([]mcpserver.TaskSummary, error) {
	return nil, nil
}
func (fakeTaskStore) GetTaskMCP(context.Context, uuid.UUID, string) (mcpserver.TaskDetailView, error) {
	return mcpserver.TaskDetailView{}, nil
}
func (fakeTaskStore) CompleteTaskMCP(context.Context, uuid.UUID, uuid.UUID, mcpserver.TaskCompleteInput) (mcpserver.TaskCompleteResult, error) {
	return mcpserver.TaskCompleteResult{TaskID: "a", CaseID: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Status: "completed"}, nil
}

func TestCreateCaseTool(t *testing.T) {
	tool := &CreateCaseTool{Store: fakeCaseStore{}}
	conn := &mcpserver.Connection{TenantID: uuid.New(), UserID: uuid.New()}
	res, err := tool.Execute(context.Background(), conn, json.RawMessage(`{"case_type":"loan","data":{"x":1}}`))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	payload := res.(map[string]any)
	if payload["case_id"] == "" {
		t.Fatalf("missing case_id")
	}
}

func TestCompleteTaskTool(t *testing.T) {
	tool := &CompleteTaskTool{Store: fakeTaskStore{}}
	conn := &mcpserver.Connection{TenantID: uuid.New(), UserID: uuid.New()}
	res, err := tool.Execute(context.Background(), conn, json.RawMessage(`{"task_id":"t1","decision":"approve"}`))
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	payload := res.(map[string]any)
	if payload["status"] != "completed" {
		t.Fatalf("expected completed, got %v", payload["status"])
	}
}
