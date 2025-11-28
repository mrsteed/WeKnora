package types

const (
	TypeChunkExtract    = "chunk:extract"
	TypeDocumentProcess = "document:process" // 文档处理任务
	TypeFAQImport       = "faq:import"       // FAQ导入任务
)

// ExtractChunkPayload represents the extract chunk task payload
type ExtractChunkPayload struct {
	TenantID uint64 `json:"tenant_id"`
	ChunkID  string `json:"chunk_id"`
	ModelID  string `json:"model_id"`
}

// DocumentProcessPayload represents the document process task payload
type DocumentProcessPayload struct {
	RequestId        string   `json:"request_id"`
	TenantID         uint64   `json:"tenant_id"`
	KnowledgeID      string   `json:"knowledge_id"`
	KnowledgeBaseID  string   `json:"knowledge_base_id"`
	FilePath         string   `json:"file_path,omitempty"` // 文件路径（文件导入时使用）
	FileName         string   `json:"file_name,omitempty"` // 文件名（文件导入时使用）
	FileType         string   `json:"file_type,omitempty"` // 文件类型（文件导入时使用）
	URL              string   `json:"url,omitempty"`       // URL（URL导入时使用）
	Passages         []string `json:"passages,omitempty"`  // 文本段落（文本导入时使用）
	EnableMultimodel bool     `json:"enable_multimodel"`
}

// FAQImportPayload represents the FAQ import task payload
type FAQImportPayload struct {
	TenantID    uint64            `json:"tenant_id"`
	TaskID      string            `json:"task_id"`
	KBID        string            `json:"kb_id"`
	KnowledgeID string            `json:"knowledge_id"`
	Entries     []FAQEntryPayload `json:"entries"`
	Mode        string            `json:"mode"`
}

// PromptTemplateStructured represents the prompt template structured
type PromptTemplateStructured struct {
	Description string      `json:"description"`
	Tags        []string    `json:"tags"`
	Examples    []GraphData `json:"examples"`
}

type GraphNode struct {
	Name       string   `json:"name,omitempty"`
	Chunks     []string `json:"chunks,omitempty"`
	Attributes []string `json:"attributes,omitempty"`
}

// GraphRelation represents the relation of the graph
type GraphRelation struct {
	Node1 string `json:"node1,omitempty"`
	Node2 string `json:"node2,omitempty"`
	Type  string `json:"type,omitempty"`
}

type GraphData struct {
	Text     string           `json:"text,omitempty"`
	Node     []*GraphNode     `json:"node,omitempty"`
	Relation []*GraphRelation `json:"relation,omitempty"`
}

// NameSpace represents the name space of the knowledge base and knowledge
type NameSpace struct {
	KnowledgeBase string `json:"knowledge_base"`
	Knowledge     string `json:"knowledge"`
}

// Labels returns the labels of the name space
func (n NameSpace) Labels() []string {
	res := make([]string, 0)
	if n.KnowledgeBase != "" {
		res = append(res, n.KnowledgeBase)
	}
	if n.Knowledge != "" {
		res = append(res, n.Knowledge)
	}
	return res
}
