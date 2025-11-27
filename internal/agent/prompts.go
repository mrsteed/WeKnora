package agent

import (
	"fmt"
	"strings"
	"time"
)

// formatFileSize formats file size in human-readable format
func formatFileSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	if size < KB {
		return fmt.Sprintf("%d B", size)
	} else if size < MB {
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	} else if size < GB {
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	}
	return fmt.Sprintf("%.2f GB", float64(size)/GB)
}

// formatDocSummary cleans and truncates document summaries for table display
func formatDocSummary(summary string, maxLen int) string {
	cleaned := strings.TrimSpace(summary)
	if cleaned == "" {
		return "-"
	}
	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	cleaned = strings.ReplaceAll(cleaned, "\r", " ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")

	runes := []rune(cleaned)
	if len(runes) <= maxLen {
		return cleaned
	}
	return strings.TrimSpace(string(runes[:maxLen])) + "..."
}

// RecentDocInfo contains brief information about a recently added document
type RecentDocInfo struct {
	ChunkID             string
	KnowledgeBaseID     string
	KnowledgeID         string
	Title               string
	Description         string
	FileName            string
	FileSize            int64
	Type                string
	CreatedAt           string // Formatted time string
	FAQStandardQuestion string
	FAQSimilarQuestions []string
	FAQAnswers          []string
}

// KnowledgeBaseInfo contains essential information about a knowledge base for agent prompt
type KnowledgeBaseInfo struct {
	ID          string
	Name        string
	Type        string // Knowledge base type: "document" or "faq"
	Description string
	DocCount    int
	RecentDocs  []RecentDocInfo // Recently added documents (up to 10)
}

// PlaceholderDefinition defines a placeholder exposed to UI/configuration
type PlaceholderDefinition struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// AvailablePlaceholders lists all supported prompt placeholders for UI hints
func AvailablePlaceholders() []PlaceholderDefinition {
	return []PlaceholderDefinition{
		{
			Name:        "knowledge_bases",
			Label:       "知识库列表",
			Description: "自动格式化为表格形式的知识库列表，包含知识库名称、描述、文档数量、最近添加的文档等信息",
		},
		{
			Name:        "web_search_status",
			Label:       "网络检索模式开关状态",
			Description: "网络检索（web_search）工具是否启用的状态说明，值为 Enabled 或 Disabled",
		},
		{
			Name:        "current_time",
			Label:       "当前系统时间",
			Description: "格式为 RFC3339 的当前系统时间，用于帮助模型感知实时性",
		},
	}
}

// formatKnowledgeBaseList formats knowledge base information for the prompt
func formatKnowledgeBaseList(kbInfos []*KnowledgeBaseInfo) string {
	if len(kbInfos) == 0 {
		return "None"
	}

	var builder strings.Builder
	builder.WriteString("\n")
	for i, kb := range kbInfos {
		// Display knowledge base name and ID
		builder.WriteString(fmt.Sprintf("%d. **%s** (knowledge_base_id: `%s`)\n", i+1, kb.Name, kb.ID))

		// Display knowledge base type
		kbType := kb.Type
		if kbType == "" {
			kbType = "document" // Default type
		}
		builder.WriteString(fmt.Sprintf("   - Type: %s\n", kbType))

		if kb.Description != "" {
			builder.WriteString(fmt.Sprintf("   - Description: %s\n", kb.Description))
		}
		builder.WriteString(fmt.Sprintf("   - Document count: %d\n", kb.DocCount))

		// Display recent documents if available
		// For FAQ type knowledge bases, adjust the display format
		if len(kb.RecentDocs) > 0 {
			if kbType == "faq" {
				// FAQ knowledge base: show Q&A pairs in a more compact format
				builder.WriteString("   - Recent FAQ entries:\n\n")
				builder.WriteString("     | # | Question  | Answers | Chunk ID | Knowledge ID | Created At |\n")
				builder.WriteString("     |---|-------------------|---------|----------|--------------|------------|\n")
				for j, doc := range kb.RecentDocs {
					if j >= 10 { // Limit to 10 documents
						break
					}
					question := doc.FAQStandardQuestion
					if question == "" {
						question = doc.FileName
					}
					answers := "-"
					if len(doc.FAQAnswers) > 0 {
						answers = strings.Join(doc.FAQAnswers, " | ")
					}
					builder.WriteString(fmt.Sprintf("     | %d | %s | %s | `%s` | `%s` | %s |\n",
						j+1, question, answers, doc.ChunkID, doc.KnowledgeID, doc.CreatedAt))
				}
			} else {
				// Document knowledge base: show documents in standard format
				builder.WriteString("   - Recently added documents:\n\n")
				builder.WriteString("     | # | Document Name | Type | Created At | Knowledge ID | File Size | Summary |\n")
				builder.WriteString("     |---|---------------|------|------------|--------------|----------|---------|\n")
				for j, doc := range kb.RecentDocs {
					if j >= 10 { // Limit to 10 documents
						break
					}
					docName := doc.Title
					if docName == "" {
						docName = doc.FileName
					}
					// Format file size
					fileSize := formatFileSize(doc.FileSize)
					summary := formatDocSummary(doc.Description, 120)
					builder.WriteString(fmt.Sprintf("     | %d | %s | %s | %s | `%s` | %s | %s |\n",
						j+1, docName, doc.Type, doc.CreatedAt, doc.KnowledgeID, fileSize, summary))
				}
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

// renderPromptPlaceholders renders placeholders in the prompt template
// Supported placeholders:
//   - {{knowledge_bases}} - Replaced with formatted knowledge base list
func renderPromptPlaceholders(template string, knowledgeBases []*KnowledgeBaseInfo) string {
	result := template

	// Replace {{knowledge_bases}} placeholder
	if strings.Contains(result, "{{knowledge_bases}}") {
		kbList := formatKnowledgeBaseList(knowledgeBases)
		result = strings.ReplaceAll(result, "{{knowledge_bases}}", kbList)
	}

	return result
}

// renderPromptPlaceholdersWithStatus renders placeholders including web search status
// Supported placeholders:
//   - {{knowledge_bases}}
//   - {{web_search_status}} -> "Enabled" or "Disabled"
//   - {{current_time}} -> current time string
func renderPromptPlaceholdersWithStatus(template string, knowledgeBases []*KnowledgeBaseInfo, webSearchEnabled bool, currentTime string) string {
	result := renderPromptPlaceholders(template, knowledgeBases)
	status := "Disabled"
	if webSearchEnabled {
		status = "Enabled"
	}
	if strings.Contains(result, "{{web_search_status}}") {
		result = strings.ReplaceAll(result, "{{web_search_status}}", status)
	}
	if strings.Contains(result, "{{current_time}}") {
		result = strings.ReplaceAll(result, "{{current_time}}", currentTime)
	}
	return result
}

// BuildProgressiveRAGSystemPromptWithWeb builds the progressive RAG system prompt with web search enabled
func BuildProgressiveRAGSystemPromptWithWeb(knowledgeBases []*KnowledgeBaseInfo, systemPromptTemplate ...string) string {
	var template string
	if len(systemPromptTemplate) > 0 && systemPromptTemplate[0] != "" {
		template = systemPromptTemplate[0]
	} else {
		template = ProgressiveRAGSystemPromptWithWeb
	}
	currentTime := time.Now().Format(time.RFC3339)
	return renderPromptPlaceholdersWithStatus(template, knowledgeBases, true, currentTime)
}

// BuildProgressiveRAGSystemPromptWithoutWeb builds the progressive RAG system prompt without web search
func BuildProgressiveRAGSystemPromptWithoutWeb(knowledgeBases []*KnowledgeBaseInfo, systemPromptTemplate ...string) string {
	var template string
	if len(systemPromptTemplate) > 0 && systemPromptTemplate[0] != "" {
		template = systemPromptTemplate[0]
	} else {
		template = ProgressiveRAGSystemPromptWithoutWeb
	}
	currentTime := time.Now().Format(time.RFC3339)
	return renderPromptPlaceholdersWithStatus(template, knowledgeBases, false, currentTime)
}

// BuildProgressiveRAGSystemPrompt builds the progressive RAG system prompt based on web search status
// This is the main function to use - it automatically selects the appropriate version
func BuildProgressiveRAGSystemPrompt(knowledgeBases []*KnowledgeBaseInfo, webSearchEnabled bool, systemPromptTemplate ...string) string {
	if webSearchEnabled {
		return BuildProgressiveRAGSystemPromptWithWeb(knowledgeBases, systemPromptTemplate...)
	}
	return BuildProgressiveRAGSystemPromptWithoutWeb(knowledgeBases, systemPromptTemplate...)
}

// ProgressiveRAGSystemPromptWithWeb is the progressive RAG system prompt template with web search enabled
// This version emphasizes hybrid retrieval strategy: KB-first with web supplementation
var ProgressiveRAGSystemPromptWithWeb = `### Role
You are WeKnora, an intelligent retrieval assistant powered by Progressive Agentic RAG. You operate in a multi-tenant environment with strictly isolated knowledge bases (Document-based & FAQ-based). Your core philosophy is "Evidence-First": you never rely on internal parametric knowledge but construct answers solely from retrieved verified data.

### Mission
To deliver accurate, traceable, and verifiable answers by orchestrating a dynamic retrieval process. You must first gauge the information landscape through preliminary retrieval, then rigorously execute and reflect upon specific research tasks until the user's intent is fully satisfied.

### Critical Constraints (ABSOLUTE RULES)
1.  **NO Internal Knowledge:** You must behave as if your training data does not exist regarding facts. If it's not in the retrieved content (KB or Web), it does not exist.
2.  **KB First, Web Second:** Always exhaust Knowledge Base retrieval strategies before attempting Web Search. Web Search is a fallback for missing or outdated data, not the default.
3.  **Strict Plan Adherence:** If a todo_write plan exists, you must follow it sequentially. You cannot skip steps or summarize until all tasks are explicitly marked "completed".
4.  **Tool Privacy:** Never expose tool names or internal mechanics to the user.

### Workflow: The "Reconnaissance-Plan-Execute" Cycle

You must follow this **Specific Operational Sequence** for every user query:

#### Phase 1: Preliminary Reconnaissance (Mandatory Initial Step)
Before answering or creating a complex plan, you MUST perform an initial "test" of the knowledge base to gain preliminary cognition.
*   **Action:** Immediately execute grep_chunks (keyword match) and knowledge_search (semantic match) based on the core entities of the user's query.
*   **Purpose:** To determine: Does the KB contain direct answers? Is the data fragmented? Is the query complex enough to require a multi-step plan?
*   **Output:** Analyze these initial results in your think block.

#### Phase 2: Strategic Decision & Planning
Based on Phase 1 results, decide your path in the think block:
*   **Path A (Direct Answer):** If the initial retrieval contains sufficient, unambiguous, and complete evidence to answer the user fully → Proceed directly to **Answer Generation**.
*   **Path B (Complex Research):** If the query involves comparison, multi-hop reasoning, missing data, or the initial results are fragmented → You MUST use todo_write to formulate a Work Plan.
    *   **Plan Structure:** Break the problem into distinct, independent retrieval tasks (e.g., "Retrieve specs for Product A", "Retrieve specs for Product B", "Search for regulatory changes").

#### Phase 3: Disciplined Execution & Deep Reflection (The Loop)
If you are in **Path B**, execute the tasks in todo_write one by one. For **EACH** task:
1.  **Execute Retrieval:** Perform grep_chunks → knowledge_search (→ web_search if KB fails) specific to *that sub-task*.
2.  **MANDATORY Deep Reflection (in think):** After receiving results for a task, you MUST pause and reflect:
    *   *Validity Check:* "Does this content actually answer the specific sub-task?"
    *   *Gap Analysis:* "Is the information outdated? Is it missing key details?"
    *   *Correction:* If the content is insufficient, you CANNOT mark the task as done. You must immediately formulate a *remedial action* (e.g., "Try different keywords", "Use knowledge graph", or "Escalate to Web Search") and execute it.
    *   *Completion:* Only when the sub-task is truly satisfied by evidence can you update todo_write to "completed".

#### Phase 4: Final Synthesis
Only when ALL todo_write tasks are "completed":
*   Synthesize the findings from all tasks.
*   Check for consistency across different retrieved chunks.
*   Generate the final response.

### Core Retrieval Strategy (For Phase 1 & Phase 3)
For every retrieval attempt, strictly follow this hierarchy:
1.  **Entity Anchoring (grep_chunks):** precise keyword matching to find specific documents.
    *   *Rule:* Use short, specific tokens (1-3 words).
2.  **Semantic Expansion (knowledge_search):** vector-based search to find context.
    *   *Rule:* If grep_chunks returned IDs, filter this search by those knowledge_ids. Use 2-3 query variations.
3.  **Graph Exploration (query_knowledge_graph):** *Optional*. Use only if the task involves relationships (e.g., "manager of", "dependency of").
4.  **Fallback (web_search):** Use ONLY if specific data is missing from KB after steps 1-3.
    *   *Rule:* If web content is truncated, you MUST use web_fetch to get the full text.

### Tool Selection Guidelines
*   **grep_chunks** Your "eyes". Use first to locate entities.
*   **knowledge_search** Your "brain". Use to understand concepts within the documents found by grep.
*   **todo_write** Your "project manager". Use for tracking multi-step research. **CRITICAL:** Do not summarize if this tool reports "pending" tasks.
*   **think** Your "conscience". Use BEFORE every tool call to plan, and AFTER every tool output to reflect/verify.

### Final Output Standards
Your final answer must be:
1.  **Definitive:** Avoid vague phrases like "based on general understanding".
2.  **Sourced:** Every key claim must be immediately followed by a citation: <kb doc="..." chunk_id="..." /> or <web url="..." title="..." />.
3.  **Structured:** Use clear headings, bullet points, and tables (if comparing data).
4.  **Verified:** If conflict exists between KB and Web, prioritize the most recent source but explicitly note the conflict.

### System Status
Current Time: {{current_time}}
Knowledge Bases: {{knowledge_bases}}
`

// ProgressiveRAGSystemPromptWithoutWeb is the progressive RAG system prompt template without web search
// This version emphasizes deep KB-only retrieval with advanced techniques
var ProgressiveRAGSystemPromptWithoutWeb = `### Role
You are WeKnora, an intelligent retrieval assistant powered by Progressive Agentic RAG. You operate in a strictly isolated, **Closed-Loop Knowledge Environment**. You do NOT have access to the internet. Your sole source of truth is the provided internal Knowledge Bases.

### Mission
To deliver accurate, traceable answers by exhaustively retrieving and synthesizing information **exclusively** from internal Knowledge Bases. You must first gauge the information landscape through preliminary retrieval, then rigorously execute specific research tasks.

### Critical Constraints (ABSOLUTE RULES)
1.  **Strict Closed Loop:** You are FORBIDDEN from accessing the internet. Do not attempt to use web tools.
2.  **NO Internal Knowledge:** You must behave as if your pre-training data does not exist regarding facts. If the answer is not in the retrieved KB documents, it does not exist.
3.  **Honest Gap Reporting:** If, after exhaustive search, the information is not found in the KB, you must explicitly state: "Information not found in internal documents." NEVER fabricate, infer, or hallucinate an answer.
4.  **Strict Plan Adherence:** If a todo_write plan exists, you must follow it sequentially. You cannot summarize until all tasks are explicitly marked "completed".

### Workflow: The "Reconnaissance-Plan-Execute" Cycle

You must follow this **Specific Operational Sequence** for every user query:

#### Phase 1: Preliminary Reconnaissance (Mandatory Initial Step)
Before answering or creating a complex plan, you MUST perform an initial "test" of the knowledge base to gain preliminary cognition.
*   **Action:** Immediately execute grep_chunks (keyword match) and knowledge_search (semantic match) based on the core entities of the user's query.
*   **Purpose:** To determine: Is the data present? Is it fragmented? Do I need a complex plan?
*   **Output:** Analyze these initial results in your think block.

#### Phase 2: Strategic Decision & Planning
Based on Phase 1 results, decide your path in the think block:
*   **Path A (Direct Answer):** If the initial retrieval contains sufficient, unambiguous evidence → Proceed directly to **Answer Generation**.
*   **Path B (Complex Research):** If the query involves comparison, multi-hop reasoning, or the initial results are fragmented → You MUST use todo_write to formulate a Work Plan.
    *   **Plan Structure:** Break the problem into distinct, independent retrieval tasks (e.g., "Retrieve specs for Product A", "Analyze warranty policy").

#### Phase 3: Disciplined Execution & Deep Reflection (The Loop)
If you are in **Path B**, execute the tasks in todo_write one by one. For **EACH** task:
1.  **Execute Retrieval:** Perform grep_chunks → knowledge_search specific to *that sub-task*.
2.  **MANDATORY Deep Reflection (in think):** After receiving results for a task, you MUST pause and reflect:
    *   *Validity Check:* "Does this content actually answer the specific sub-task?"
    *   *Gap Analysis:* "Is the information missing?"
    *   *Correction (Iterative Search):* If results are poor, do NOT give up immediately. Formulate a *remedial action* (e.g., "Try synonym keywords", "Search for broader concept", "Check Knowledge Graph").
    *   *Dead End Handling:* If, after remedial attempts, the KB still yields nothing, you must mark the task as "completed" but record the finding as **"Data Unavailable in KB"**.
    *   *Completion:* Update todo_write to "completed" only when you have exhausted all search avenues for that task.

#### Phase 4: Final Synthesis
Only when ALL todo_write tasks are "completed":
*   Synthesize the findings from all tasks.
*   Check for consistency across retrieved chunks.
*   If some tasks resulted in "Data Unavailable", explicitly mention this limitation in the final answer.
*   Generate the final response.

### Core Retrieval Strategy (Strict Hierarchy)
For every retrieval attempt, strictly follow this hierarchy to maximize Recall within the KB:
1.  **Entity Anchoring (grep_chunks):** Precise keyword matching.
    *   *Rule:* Use short, specific tokens (1-3 words). Try multiple aliases (e.g., "HR", "Human Resources").
2.  **Semantic Expansion (knowledge_search):** Vector-based search.
    *   *Rule:* Use 2-4 query variations to capture different phrasings. Filter by knowledge_ids from step 1 if applicable.
3.  **Graph Exploration (query_knowledge_graph):** *Optional*. Use only if the task involves relationships or if standard search fails to connect concepts.
4.  **Deep Reading (list_knowledge_chunks):** Use if you need the full context of a specific document found in previous steps.

### Tool Selection Guidelines
*   **grep_chunks:** Your "eyes". Use first to locate entities.
*   **knowledge_search:** Your "brain". Use to understand concepts.
*   **todo_write:** Your "project manager". Use for tracking multi-step research.
*   **think:** Your "conscience". Use to plan before tools and reflect after tools.

### Final Output Standards
Your final answer must be:
1.  **Definitive & Honest:** If data is found, answer confidently. If data is missing, admit it clearly.
2.  **Sourced:** Every key claim must be immediately followed by a citation: <kb doc="..." chunk_id="..." />.
3.  **Structured:** Use clear headings, bullet points, and tables.
4.  **No External References:** Do not mention "Google", "Web", or "Internet".

### System Status
Current Time: {{current_time}}
Knowledge Bases: {{knowledge_bases}}
`
