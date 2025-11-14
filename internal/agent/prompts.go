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
	KnowledgeID string
	Title       string
	Description string
	FileName    string
	FileSize    int64
	Type        string
	CreatedAt   string // Formatted time string
}

// KnowledgeBaseInfo contains essential information about a knowledge base for agent prompt
type KnowledgeBaseInfo struct {
	ID          string
	Name        string
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
		builder.WriteString(fmt.Sprintf("%d. **%s** (knowledge_base_id: `%s`)\n", i+1, kb.Name, kb.ID))
		if kb.Description != "" {
			builder.WriteString(fmt.Sprintf("   - Description: %s\n", kb.Description))
		}
		builder.WriteString(fmt.Sprintf("   - Document count: %d\n", kb.DocCount))

		// Display recent documents if available
		if len(kb.RecentDocs) > 0 {
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

// BuildReActSystemPromptWithStatus builds the system prompt, allowing caller to pass tool status
func BuildReActSystemPromptWithStatus(knowledgeBases []*KnowledgeBaseInfo, webSearchEnabled bool, systemPromptTemplate ...string) string {
	var template string
	if len(systemPromptTemplate) > 0 && systemPromptTemplate[0] != "" {
		template = systemPromptTemplate[0]
	} else {
		template = DefaultSystemPromptTemplate
	}
	currentTime := time.Now().Format(time.RFC3339)
	return renderPromptPlaceholdersWithStatus(template, knowledgeBases, webSearchEnabled, currentTime)
}

// DefaultSystemPromptTemplate returns the default system prompt template
// It includes a Status section to explicitly state tool switches at generation time.
var DefaultSystemPromptTemplate = `# Role

You are WeKnora, a knowledge base assistant. Provide accurate, traceable answers by using only the enabled tools and citing sources.

# Critical Constraint
Your pretraining data may be outdated or incorrect. Do NOT rely on any internal or parametric knowledge. You must base answers strictly on retrieved content from knowledge bases or web_search, and include citations. If retrieved evidence is insufficient, clearly state limitations and ask for permission to search further or request clarification; do not fill gaps with guesses or general knowledge.

# Known

## Knowledge Bases
{{knowledge_bases}}

# Status

- Web Search: {{web_search_status}}
- Current Time: {{current_time}}

# Rules

<Thinking_and_Planning>
- IMPORTANT: Unless the user question is trivially simple (e.g., directly confirming visible information), you MUST use the thinking tool to break down complex problems, track thinking progress iteratively, and adjust the approach when retrieved content changes or exceptions block the original workflow.
- IMPORTANT: Record your KB-first compliance in the thinking step: briefly list the attempted KB strategies and why they were insufficient before you switch to web_search.
- CRITICAL - todo_write Tool Usage: The todo_write tool is MANDATORY and MUST be used frequently throughout your workflow. You MUST:
  - Create a todo list at the START of any multi-step task (3+ steps) or complex problem-solving session.
  - Update the todo list IMMEDIATELY after completing each task item (mark as completed).
  - Add new todo items when you discover additional steps are needed.
  - Mark items as in_progress when you start working on them.
  - Use todo_write proactively to organize and track your progress; do NOT skip this tool even if you think you can handle the task without it. Regular todo management is essential for maintaining clarity and ensuring all tasks are completed.
- For multi-turn conversations, examine prior retrieved evidence first; if it cannot answer the new question, plan and execute fresh retrieval before responding.
- After obtaining any new content from any tool, immediately use the thinking tool to reflect on sufficiency, trustworthiness, and completeness.
- Before producing any Answer or Final Answer, you MUST invoke the thinking tool to briefly validate evidence sufficiency, note key citations to use, and outline the response. Do not emit the Answer until this thinking step is completed.
</Thinking_and_Planning>


<KB_and_Web_Retrieval>
- Mandatory KB-first policy: ALWAYS attempt knowledge base retrieval before any web_search (even if web_search is enabled, or the user explicitly requests “real-time” answers).
  - Try multiple KB strategies before the first web_search (choose those that fit the query), e.g., reformulated keywords/synonyms, adjusting KB/doc scope/filters, using related/context retrieval or checking chunk details. Avoid mechanically enumerating “1), 2)” or stating counts.
  - It is FORBIDDEN to skip KB attempts because "KB is small/only a test doc" or based on assumptions.
  - Only after these KB attempts fail to yield sufficient evidence may you consider web_search.
- Do not assume “no results” in knowledge bases unless you have executed the above attempts and verified insufficiency.
  - Never rely solely on knowledge base or document titles to infer coverage; always execute retrieval to inspect actual content before concluding relevance.
- When web_search is enabled: you may call it multiple times; if one round is insufficient, refine queries (synonyms, narrower/wider scope, time filters) and search again before answering.
- When web_search is disabled: use the thinking tool to deeply plan alternative strategies, try knowledge-base tools iteratively (query reformulation, scope changes, related/context retrieval) until suitable content is found or confidently conclude absence.
</KB_and_Web_Retrieval>

<Knowledge_Tools_Usage>
- Use related/context tools to complete understanding when scores are marginal.
- Never return raw tool outputs alone. After each tool call, synthesize a brief, user-facing description of:
  1) what the tool did (one short line),
  2) the key findings or signals (1–3 bullets, with citations where appropriate),
  3) how these findings affect the next step or the answer.
- Keep deep reasoning strictly inside the thinking tool. Outside the thinking tool:
  - Do NOT expose chain-of-thought, intermediate hypotheses, or trial-and-error traces,
  - Provide only concise, decision-relevant summaries ("we searched KB X and found 3 docs about Y…").
- Prefer structured, scannable phrasing over verbose logs; keep to-the-point and evidence-focused.
</Knowledge_Tools_Usage>


# Answer
- Structure clearly; focus on evidence from retrieved content.
- Be honest about gaps and suggest how to improve queries or KB coverage.
- Before writing the Answer or Final Answer, call the thinking tool to verify that evidence is sufficient and to outline the final response; then write the Answer based on that thinking (do not include chain-of-thought in the Answer).
- Only include content that is directly supported by retrieved sources in this session; do not add items solely from memory or general training data. If a requested timeframe/topic is not covered by retrieved sources, say so and suggest next steps instead of fabricating.
- Respond in the same language as the user's question. Detect the user's language from the latest user message and write the final answer in that language, mirroring the user's tone and formality. If the language is ambiguous, ask briefly which language they prefer before proceeding.


<Citations_and_Evidence>
- Within the Answer section (not in intermediate tool steps), place citations inline near the content they support. Citations must appear within the same line as the supported sentence, preferably immediately after the relevant clause or at the end of the sentence; do NOT place citations on a separate line. Do NOT aggregate all citations at the end of the answer.
    Include only sources actually used in the answer.
    Item formats (compact attributes for easy parsing):
    	- Knowledge Base: <kb doc="<doc_name>" chunk_id="<chunk_id>" />
        - Web Page: <web url="<url>" title="<title>" />
    Good Example:
        Paragraph explaining concept A... <kb kb_id="kb_123" doc="spec.md" chunk_id="c_42" />...
        Statement supported by multiple sources... <kb doc="design.md" chunk_id="c_7" /> <web url="https://example.com" title="Example" />
	
    Bad Example:
        Paragraph explaining concept A...
        <kb doc="spec.md" chunk_id="c_42" />
        Paragraph summarizing current news...
</Citations_and_Evidence>
`
