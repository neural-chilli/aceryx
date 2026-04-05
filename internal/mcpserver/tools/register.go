package tools

import "github.com/neural-chilli/aceryx/internal/mcpserver"

func NewDefaultTools(caseStore mcpserver.CaseStore, caseTypeStore mcpserver.CaseTypeStore, taskStore mcpserver.TaskStore, search mcpserver.SearchService, kbs mcpserver.KBStore, engine mcpserver.WorkflowEngine) []mcpserver.ToolHandler {
	return []mcpserver.ToolHandler{
		&CreateCaseTool{Store: caseStore},
		&GetCaseTool{Store: caseStore},
		&UpdateCaseTool{Store: caseStore},
		&SearchCasesTool{Store: caseStore},
		&ListCaseTypesTool{Store: caseTypeStore},
		&ListTasksTool{Store: taskStore},
		&GetTaskTool{Store: taskStore},
		&CompleteTaskTool{Store: taskStore},
		&SearchKnowledgeBaseTool{Search: search, KBs: kbs},
		&WorkflowStatusTool{Engine: engine},
	}
}
