package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/client"
)

// Colors for terminal output
const (
	ColorReset   = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
	ColorBold    = "\033[1m"
)

// Config holds the configuration
type Config struct {
	BaseURL         string
	Token           string
	KnowledgeBaseID string
}

// CLI represents the command line interface
type CLI struct {
	client           *client.Client
	config           *Config
	currentSessionID string
	agentSession     *client.AgentSession
}

func main() {
	baseURL := flag.String("url", "http://localhost:8080", "WeKnora API base URL")
	token := flag.String("token", "", "API authentication token")
	kbID := flag.String("kb", "", "Knowledge base ID")
	sessionID := flag.String("session", "", "Existing session ID (optional)")

	flag.Parse()

	config := &Config{
		BaseURL:         *baseURL,
		Token:           *token,
		KnowledgeBaseID: *kbID,
	}

	cli := NewCLI(config)

	// Use existing session if provided
	if *sessionID != "" {
		cli.currentSessionID = *sessionID
		cli.agentSession = cli.client.NewAgentSession(*sessionID)
		fmt.Printf("%s✓ Using existing session: %s%s\n\n", ColorGreen, *sessionID, ColorReset)
	}

	cli.Run()
}

// NewCLI creates a new CLI instance
func NewCLI(config *Config) *CLI {
	clientOptions := []client.ClientOption{
		client.WithTimeout(120 * time.Second),
	}
	if config.Token != "" {
		clientOptions = append(clientOptions, client.WithToken(config.Token))
	}

	return &CLI{
		client: client.NewClient(config.BaseURL, clientOptions...),
		config: config,
	}
}

// Run starts the interactive CLI
func (cli *CLI) Run() {
	cli.printWelcome()

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print(cli.getPrompt())

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("%sError reading input: %v%s\n", ColorRed, err, ColorReset)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if !cli.handleCommand(input) {
			break
		}
	}

	fmt.Printf("\n%sGoodbye!%s\n", ColorCyan, ColorReset)
}

func (cli *CLI) getPrompt() string {
	if cli.currentSessionID != "" {
		return fmt.Sprintf("%s[Session: %s]%s > ", ColorBlue, cli.currentSessionID[:8], ColorReset)
	}
	return fmt.Sprintf("%s[No Session]%s > ", ColorYellow, ColorReset)
}

func (cli *CLI) printWelcome() {
	fmt.Printf("\n%s╔════════════════════════════════════════════════════════════╗%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s║         WeKnora Agent QA Testing Tool                     ║%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s╚════════════════════════════════════════════════════════════╝%s\n\n", ColorCyan, ColorReset)
	fmt.Printf("%sAvailable commands:%s\n", ColorBold, ColorReset)
	fmt.Printf("  %snew%s           - Create a new session\n", ColorGreen, ColorReset)
	fmt.Printf("  %sask <query>%s  - Ask agent a question\n", ColorGreen, ColorReset)
	fmt.Printf("  %sinfo%s         - Show current session info\n", ColorGreen, ColorReset)
	fmt.Printf("  %ssessions%s     - List all sessions\n", ColorGreen, ColorReset)
	fmt.Printf("  %sswitch <id>%s  - Switch to another session\n", ColorGreen, ColorReset)
	fmt.Printf("  %shelp%s         - Show this help message\n", ColorGreen, ColorReset)
	fmt.Printf("  %sexit/quit%s    - Exit the program\n\n", ColorGreen, ColorReset)
}

func (cli *CLI) handleCommand(input string) bool {
	parts := strings.SplitN(input, " ", 2)
	command := strings.ToLower(parts[0])

	switch command {
	case "help":
		cli.printWelcome()
	case "new":
		cli.createNewSession()
	case "ask":
		if len(parts) < 2 {
			fmt.Printf("%sUsage: ask <your question>%s\n", ColorRed, ColorReset)
			return true
		}
		cli.askQuestion(parts[1])
	case "info":
		cli.showSessionInfo()
	case "sessions":
		cli.listSessions()
	case "switch":
		if len(parts) < 2 {
			fmt.Printf("%sUsage: switch <session_id>%s\n", ColorRed, ColorReset)
			return true
		}
		cli.switchSession(parts[1])
	case "exit", "quit":
		return false
	default:
		fmt.Printf("%sUnknown command: %s. Type 'help' for available commands.%s\n", ColorRed, command, ColorReset)
	}

	return true
}

func (cli *CLI) createNewSession() {
	if cli.config.KnowledgeBaseID == "" {
		fmt.Printf("%sError: Knowledge base ID is required. Use -kb flag.%s\n", ColorRed, ColorReset)
		return
	}

	fmt.Printf("%sCreating new session...%s\n", ColorYellow, ColorReset)

	ctx := context.Background()
	request := &client.CreateSessionRequest{
		KnowledgeBaseID: cli.config.KnowledgeBaseID,
		SessionStrategy: &client.SessionStrategy{
			MaxRounds:        10,
			EnableRewrite:    false,
			FallbackStrategy: "default",
			EmbeddingTopK:    10,
		},
	}

	session, err := cli.client.CreateSession(ctx, request)
	if err != nil {
		fmt.Printf("%s✗ Failed to create session: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	cli.currentSessionID = session.ID
	cli.agentSession = cli.client.NewAgentSession(session.ID)

	fmt.Printf("%s✓ Session created successfully!%s\n", ColorGreen, ColorReset)
	fmt.Printf("  Session ID: %s%s%s\n", ColorCyan, session.ID, ColorReset)
	fmt.Printf("  Knowledge Base: %s%s%s\n", ColorCyan, session.KnowledgeBaseID, ColorReset)
	fmt.Printf("  Created At: %s%s%s\n\n", ColorCyan, session.CreatedAt, ColorReset)
}

func (cli *CLI) askQuestion(query string) {
	if cli.agentSession == nil {
		fmt.Printf("%sError: No active session. Create one with 'new' command.%s\n", ColorRed, ColorReset)
		return
	}

	fmt.Printf("\n%s╔═══ Agent Processing ═══╗%s\n", ColorCyan, ColorReset)
	fmt.Printf("%sQuery: %s%s%s\n\n", ColorBold, ColorWhite, query, ColorReset)

	ctx := context.Background()

	var (
		thinkingContent string
		finalAnswer     string
		references      []*client.SearchResult
		toolCalls       []string
		reflections     []string
	)

	startTime := time.Now()

	err := cli.agentSession.Ask(ctx, query, func(resp *client.AgentStreamResponse) error {
		switch resp.ResponseType {
		case client.AgentResponseTypeThinking:
			thinkingContent += resp.Content
			if resp.Done {
				fmt.Printf("%s💭 Thinking:%s %s\n\n", ColorYellow, ColorReset, thinkingContent)
				thinkingContent = ""
			}

		case client.AgentResponseTypeToolCall:
			toolName := "Unknown"
			if resp.Data != nil {
				if name, ok := resp.Data["tool_name"].(string); ok {
					toolName = name
				}
			}
			toolCalls = append(toolCalls, toolName)
			fmt.Printf("%s🔧 Tool Call:%s %s\n", ColorMagenta, ColorReset, toolName)
			if resp.Data != nil {
				if args, ok := resp.Data["arguments"]; ok {
					fmt.Printf("   Arguments: %v\n", args)
				}
			}

		case client.AgentResponseTypeToolResult:
			fmt.Printf("%s✓ Tool Result:%s\n", ColorGreen, ColorReset)
			fmt.Printf("   %s\n\n", resp.Content)

		case client.AgentResponseTypeReferences:
			if resp.KnowledgeReferences != nil {
				references = append(references, resp.KnowledgeReferences...)
				fmt.Printf("%s📚 Knowledge References:%s Found %d reference(s)\n", ColorCyan, ColorReset, len(resp.KnowledgeReferences))
				for i, ref := range resp.KnowledgeReferences {
					fmt.Printf("   %d. [Score: %.3f] %s\n", i+1, ref.Score, truncateString(ref.Content, 80))
					fmt.Printf("      Knowledge: %s (Chunk: %d)\n", ref.KnowledgeTitle, ref.ChunkIndex)
				}
				fmt.Println()
			}

		case client.AgentResponseTypeAnswer:
			finalAnswer += resp.Content
			if resp.Done {
				fmt.Printf("%s📝 Final Answer:%s\n", ColorGreen, ColorReset)
				fmt.Printf("%s\n\n", finalAnswer)
			}

		case client.AgentResponseTypeReflection:
			if resp.Done && resp.Content != "" {
				reflections = append(reflections, resp.Content)
				fmt.Printf("%s🤔 Reflection:%s %s\n\n", ColorBlue, ColorReset, resp.Content)
			}

		case client.AgentResponseTypeError:
			fmt.Printf("%s✗ Error:%s %s\n\n", ColorRed, ColorReset, resp.Content)
		}

		return nil
	})
	if err != nil {
		fmt.Printf("%s✗ Agent QA failed: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	duration := time.Since(startTime)

	// Print summary
	fmt.Printf("%s╔═══ Summary ═══╗%s\n", ColorCyan, ColorReset)
	fmt.Printf("Duration: %s%.2fs%s\n", ColorCyan, duration.Seconds(), ColorReset)
	fmt.Printf("Tool Calls: %s%d%s", ColorCyan, len(toolCalls), ColorReset)
	if len(toolCalls) > 0 {
		fmt.Printf(" (%s)", strings.Join(toolCalls, ", "))
	}
	fmt.Println()
	fmt.Printf("References: %s%d%s\n", ColorCyan, len(references), ColorReset)
	fmt.Printf("Reflections: %s%d%s\n", ColorCyan, len(reflections), ColorReset)
	fmt.Printf("%s╚═══════════════╝%s\n\n", ColorCyan, ColorReset)
}

func (cli *CLI) showSessionInfo() {
	if cli.currentSessionID == "" {
		fmt.Printf("%sNo active session.%s\n", ColorYellow, ColorReset)
		return
	}

	ctx := context.Background()
	session, err := cli.client.GetSession(ctx, cli.currentSessionID)
	if err != nil {
		fmt.Printf("%s✗ Failed to get session info: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	fmt.Printf("\n%s╔═══ Session Information ═══╗%s\n", ColorCyan, ColorReset)
	fmt.Printf("Session ID: %s%s%s\n", ColorWhite, session.ID, ColorReset)
	fmt.Printf("Title: %s%s%s\n", ColorWhite, session.Title, ColorReset)
	fmt.Printf("Knowledge Base ID: %s%s%s\n", ColorWhite, session.KnowledgeBaseID, ColorReset)
	fmt.Printf("Max Rounds: %s%d%s\n", ColorWhite, session.MaxRounds, ColorReset)
	fmt.Printf("Enable Rewrite: %s%v%s\n", ColorWhite, session.EnableRewrite, ColorReset)
	fmt.Printf("Summary Model: %s%s%s\n", ColorWhite, session.SummaryModelID, ColorReset)
	fmt.Printf("Created At: %s%s%s\n", ColorWhite, session.CreatedAt, ColorReset)
	fmt.Printf("Updated At: %s%s%s\n", ColorWhite, session.UpdatedAt, ColorReset)
	fmt.Printf("%s╚═══════════════════════════╝%s\n\n", ColorCyan, ColorReset)
}

func (cli *CLI) listSessions() {
	ctx := context.Background()
	sessions, total, err := cli.client.GetSessionsByTenant(ctx, 1, 10)
	if err != nil {
		fmt.Printf("%s✗ Failed to list sessions: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	fmt.Printf("\n%s╔═══ Sessions (Total: %d) ═══╗%s\n", ColorCyan, total, ColorReset)
	for i, session := range sessions {
		marker := " "
		if session.ID == cli.currentSessionID {
			marker = "►"
		}
		fmt.Printf("%s%s %d. %s%s\n", ColorGreen, marker, i+1, session.ID, ColorReset)
		fmt.Printf("     Title: %s\n", session.Title)
		fmt.Printf("     KB: %s\n", session.KnowledgeBaseID)
		fmt.Printf("     Created: %s\n", session.CreatedAt)
		fmt.Println()
	}
	fmt.Printf("%s╚════════════════════════════╝%s\n\n", ColorCyan, ColorReset)
}

func (cli *CLI) switchSession(sessionID string) {
	ctx := context.Background()
	session, err := cli.client.GetSession(ctx, sessionID)
	if err != nil {
		fmt.Printf("%s✗ Failed to switch session: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	cli.currentSessionID = session.ID
	cli.agentSession = cli.client.NewAgentSession(session.ID)

	fmt.Printf("%s✓ Switched to session: %s%s\n", ColorGreen, session.ID, ColorReset)
}

func truncateString(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
