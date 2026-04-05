package ai

type ListResponse struct {
	Items []*AIComponentDef `json:"items"`
}

type CategoriesResponse struct {
	Items []ComponentCategory `json:"items"`
}

type UpsertRequest struct {
	Definition *AIComponentDef `json:"definition"`
}
