package router

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	filesvc "github.com/Tencent/WeKnora/internal/application/service/file"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/dig"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/handler/session"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"

	_ "github.com/Tencent/WeKnora/docs" // swagger docs
)

// RouterParams 路由参数
type RouterParams struct {
	dig.In

	Config                   *config.Config
	FileService              interfaces.FileService
	UserService              interfaces.UserService
	KBService                interfaces.KnowledgeBaseService
	KnowledgeService         interfaces.KnowledgeService
	ChunkService             interfaces.ChunkService
	SessionService           interfaces.SessionService
	MessageService           interfaces.MessageService
	ModelService             interfaces.ModelService
	EvaluationService        interfaces.EvaluationService
	KBHandler                *handler.KnowledgeBaseHandler
	KnowledgeHandler         *handler.KnowledgeHandler
	TenantHandler            *handler.TenantHandler
	TenantMemberHandler      *handler.TenantMemberHandler
	TenantInvitationHandler  *handler.TenantInvitationHandler
	AuditLogHandler          *handler.AuditLogHandler
	TenantService            interfaces.TenantService
	TenantMemberService      interfaces.TenantMemberService
	KBShareService           interfaces.KBShareService
	AgentShareService        interfaces.AgentShareService
	ChunkHandler             *handler.ChunkHandler
	SessionHandler           *session.Handler
	MessageHandler           *handler.MessageHandler
	ModelHandler             *handler.ModelHandler
	EvaluationHandler        *handler.EvaluationHandler
	AuthHandler              *handler.AuthHandler
	InitializationHandler    *handler.InitializationHandler
	SystemHandler            *handler.SystemHandler
	MCPServiceHandler        *handler.MCPServiceHandler
	WebSearchHandler         *handler.WebSearchHandler
	WebSearchProviderHandler *handler.WebSearchProviderHandler
	VectorStoreHandler       *handler.VectorStoreHandler
	FAQHandler               *handler.FAQHandler
	TagHandler               *handler.TagHandler
	CustomAgentHandler       *handler.CustomAgentHandler
	SkillHandler             *handler.SkillHandler
	OrganizationHandler      *handler.OrganizationHandler
	IMHandler                *handler.IMHandler
	DataSourceHandler        *handler.DataSourceHandler
	WeKnoraCloudHandler      *handler.WeKnoraCloudHandler
	WikiPageHandler          *handler.WikiPageHandler
	OrgTreeHandler           *handler.OrgTreeHandler
	ExportHandler            *handler.ExportHandler
}

// NewRouter 创建新的路由
func NewRouter(params RouterParams) *gin.Engine {
	r := gin.New()
	r.ContextWithFallback = true

	// CORS 中间件应放在最前面
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key", "X-Request-ID", "X-Share-Session-Token"},
		ExposeHeaders:    []string{"Content-Length", "Access-Control-Allow-Origin"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 基础中间件（不需要认证）
	r.Use(middleware.RequestID())
	r.Use(middleware.Language())
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())
	r.Use(middleware.ErrorHandler())

	// 健康检查（不需要认证）
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Swagger API 文档。
	// 默认在非生产环境启用；生产环境可通过 APP_ENV/ENV 识别，或用
	// WEKNORA_SWAGGER_ENABLED/SWAGGER_ENABLED 显式覆盖。
	if swaggerEnabled, reason := resolveSwaggerEnabled(); swaggerEnabled {
		logger.Infof(context.Background(), "[swagger] enabled (%s)", reason)
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler,
			ginSwagger.DefaultModelsExpandDepth(-1), // 默认折叠 Models
			ginSwagger.DocExpansion("list"),         // 展开模式: "list"(展开标签), "full"(全部展开), "none"(全部折叠)
			ginSwagger.DeepLinking(true),            // 启用深度链接
			ginSwagger.PersistAuthorization(true),   // 持久化认证信息
		))
	} else {
		logger.Infof(context.Background(), "[swagger] disabled (%s)", reason)
	}

	// 前端静态文件（仅 Lite 版本内嵌前端）
	if handler.Edition == "lite" {
		serveFrontendStatic(r)
	}

	// IM 回调路由（在认证中间件之前注册，使用各平台自身的签名验证）
	RegisterIMRoutes(r, params.IMHandler)
	RegisterPublicAgentPageShareRoutes(r, params.CustomAgentHandler)
	RegisterPublicAgentPageShareChatRoutes(r, params.SessionHandler)
	RegisterPublicAgentPageShareExportRoutes(r, params.ExportHandler)

	// 认证中间件
	r.Use(middleware.Auth(params.TenantService, params.UserService, params.TenantMemberService, params.Config))

	// 文件服务：统一代理本地/MinIO/COS/TOS存储后端（需要认证）
	serveFiles(r, params.FileService)

	// Presigned file access: no auth required, signature-verified.
	servePresignedFiles(r, params.TenantService)

	// Diagnostic preview of presigned URLs (Admin only, behind auth middleware).
	servePresignedPreview(r, params.Config)

	// 添加OpenTelemetry追踪中间件
	// r.Use(middleware.TracingMiddleware())

	// Langfuse observability — only active when LANGFUSE_* env vars are set.
	// The middleware is registered unconditionally; when disabled it's a no-op.
	r.Use(langfuse.GinMiddleware())
	g := newRBACGuards(
		params.Config,
		params.KBHandler,
		params.CustomAgentHandler,
		params.KnowledgeHandler,
		params.ChunkHandler,
		params.WikiPageHandler,
		params.KBService,
		params.KnowledgeService,
		params.ChunkService,
		params.KBShareService,
		params.AgentShareService,
	)

	// 需要认证的API路由
	v1 := r.Group("/api/v1")
	{
		RegisterAuthRoutes(v1, params.AuthHandler)
		RegisterTenantRoutes(v1, g, params.Config, params.TenantHandler, params.TenantMemberHandler, params.TenantInvitationHandler, params.AuditLogHandler)
		RegisterKnowledgeBaseRoutes(v1, params.KBHandler)
		RegisterKnowledgeTagRoutes(v1, params.TagHandler)
		RegisterKnowledgeRoutes(v1, g, params.KnowledgeHandler)
		RegisterFAQRoutes(v1, params.FAQHandler)
		RegisterChunkRoutes(v1, params.ChunkHandler)
		RegisterSessionRoutes(v1, params.SessionHandler)
		RegisterChatRoutes(v1, params.SessionHandler)
		RegisterMessageRoutes(v1, params.MessageHandler)
		RegisterModelRoutes(v1, params.ModelHandler)
		RegisterEvaluationRoutes(v1, params.EvaluationHandler)
		RegisterInitializationRoutes(v1, params.InitializationHandler)
		RegisterMCPServiceRoutes(v1, params.MCPServiceHandler)
		RegisterWebSearchRoutes(v1, params.WebSearchHandler)
		RegisterWebSearchProviderRoutes(v1, params.WebSearchProviderHandler)
		RegisterVectorStoreRoutes(v1, params.VectorStoreHandler)
		RegisterCustomAgentRoutes(v1, params.CustomAgentHandler)
		RegisterSkillRoutes(v1, params.SkillHandler)
		RegisterOrganizationRoutes(v1, params.OrganizationHandler)
		RegisterIMChannelRoutes(v1, params.IMHandler)
		RegisterDataSourceRoutes(v1, params.DataSourceHandler)
		RegisterWeKnoraCloudRoutes(v1, params.WeKnoraCloudHandler)
		RegisterWikiPageRoutes(v1, params.WikiPageHandler)
		RegisterChunkerDebugRoutes(v1)
		RegisterExportRoutes(v1, params.ExportHandler)

		// System info routes (accessible by all authenticated users)
		RegisterSystemRoutes(v1, params.SystemHandler)

		// Org-tree management routes (accessible by super admin and org admin, permission enforced in handler)
		RegisterOrgTreeRoutes(v1, params.OrgTreeHandler)

		// Super admin routes (require super admin privileges)
		superAdmin := v1.Group("", middleware.RequireSuperAdmin())
		{
			// Super admin only org-tree operations
			RegisterOrgTreeSuperAdminRoutes(superAdmin, params.OrgTreeHandler)
			// Model write operations (super admin only)
			RegisterModelWriteRoutes(superAdmin, params.ModelHandler)
			// Note: Tenant KV write routes moved to RegisterTenantRoutes to allow
			// normal users to update their own tenant's configuration
		}

		// User org-tree membership route (accessible by all authenticated users)
		v1.GET("/my-organizations", params.OrgTreeHandler.GetMyOrganizations)
	}

	return r
}

// RegisterChunkerDebugRoutes wires the read-only chunker preview endpoint
// used by the KB editor's debug panel. Stateless — uses no service deps.
func RegisterChunkerDebugRoutes(r *gin.RouterGroup) {
	r.POST("/chunker/preview", handler.PreviewChunking)
}

// RegisterChunkRoutes 注册分块相关的路由
func RegisterChunkRoutes(r *gin.RouterGroup, handler *handler.ChunkHandler) {
	// 分块路由组
	chunks := r.Group("/chunks")
	{
		// 获取分块列表
		chunks.GET("/:knowledge_id", handler.ListKnowledgeChunks)
		// 通过chunk_id获取单个chunk（不需要knowledge_id）
		chunks.GET("/by-id/:id", handler.GetChunkByIDOnly)
		// 删除分块
		chunks.DELETE("/:knowledge_id/:id", handler.DeleteChunk)
		// 删除知识下的所有分块
		chunks.DELETE("/:knowledge_id", handler.DeleteChunksByKnowledgeID)
		// 更新分块信息
		chunks.PUT("/:knowledge_id/:id", handler.UpdateChunk)
		// 删除单个生成的问题（通过问题ID）
		chunks.DELETE("/by-id/:id/questions", handler.DeleteGeneratedQuestion)
	}
}

// RegisterKnowledgeRoutes 注册知识相关的路由
func RegisterKnowledgeRoutes(r *gin.RouterGroup, g *rbacGuards, handler *handler.KnowledgeHandler) {
	// 知识库下的知识路由组
	kb := r.Group("/knowledge-bases/:id/knowledge")
	{
		// 从文件创建知识
		kb.POST("/file", handler.CreateKnowledgeFromFile)
		// 从URL创建知识（支持网页URL和文件URL，传 file_name/file_type 或 URL 含已知扩展名时自动切换为文件下载模式）
		kb.POST("/url", handler.CreateKnowledgeFromURL)
		// 手工 Markdown 录入
		kb.POST("/manual", handler.CreateManualKnowledge)
		// 获取知识库下的知识列表
		kb.GET("", handler.ListKnowledge)
		// 清空知识库下的所有知识
		kb.DELETE("", handler.ClearKnowledgeBaseContents)
	}

	// 知识路由组
	k := r.Group("/knowledge")
	{
		// Cross-knowledge endpoints (no :id) can't be gated on a single
		// KB — they accept arbitrary knowledge IDs and the handler must
		// fan out the access check itself. So /batch and /search keep
		// the role-only floor; /move and /batch-delete stay Contributor.
		k.GET("/batch", g.Viewer(), handler.GetKnowledgeBatch)
		k.GET("/:id", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.GetKnowledge)
		k.GET("/:id/stages", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.GetKnowledgeSpans)
		k.GET("/:id/spans", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.GetKnowledgeSpans)
		k.DELETE("/:id", g.OwnedKnowledgeKBOrAdmin(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.DeleteKnowledge)
		k.PUT("/:id", g.OwnedKnowledgeKBOrAdmin(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.UpdateKnowledge)
		k.PUT("/manual/:id", g.OwnedKnowledgeKBOrAdmin(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.UpdateManualKnowledge)
		k.POST("/:id/reparse", g.OwnedKnowledgeKBOrAdmin(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.ReparseKnowledge)
		k.POST("/:id/cancel-parse", g.OwnedKnowledgeKBOrAdmin(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.CancelKnowledgeParse)
		k.GET("/:id/download", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.DownloadKnowledgeFile)
		k.GET("/:id/preview", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.PreviewKnowledgeFile)
		k.PUT("/image/:id/:chunk_id", g.OwnedKnowledgeKBOrAdmin(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.UpdateImageInfo)
		// Batch / cross-KB ops stay Contributor-gated: there is no
		// single owning KB to walk back to. A future PR could add a
		// "must own every targeted KB" guard if the requirement
		// surfaces.
		k.PUT("/tags", g.Contributor(), handler.UpdateKnowledgeTagBatch)
		k.GET("/search", g.Viewer(), handler.SearchKnowledge)
		k.POST("/batch-delete", g.Contributor(), handler.BatchDeleteKnowledge)
		k.POST("/move", g.Contributor(), handler.MoveKnowledge)
		k.GET("/move/progress/:task_id", g.Viewer(), handler.GetKnowledgeMoveProgress)
	}
}

// RegisterFAQRoutes 注册 FAQ 相关路由
func RegisterFAQRoutes(r *gin.RouterGroup, handler *handler.FAQHandler) {
	if handler == nil {
		return
	}
	faq := r.Group("/knowledge-bases/:id/faq")
	{
		faq.GET("/entries", handler.ListEntries)
		faq.GET("/entries/export", handler.ExportEntries)
		faq.GET("/entries/:entry_id", handler.GetEntry)
		faq.POST("/entries", handler.UpsertEntries)
		faq.POST("/entry", handler.CreateEntry)
		faq.PUT("/entries/:entry_id", handler.UpdateEntry)
		faq.POST("/entries/:entry_id/similar-questions", handler.AddSimilarQuestions)
		// Unified batch update API - supports is_enabled, is_recommended, tag_id
		faq.PUT("/entries/fields", handler.UpdateEntryFieldsBatch)
		faq.PUT("/entries/tags", handler.UpdateEntryTagBatch)
		faq.DELETE("/entries", handler.DeleteEntries)
		faq.POST("/search", handler.SearchFAQ)
		// FAQ import result display status
		faq.PUT("/import/last-result/display", handler.UpdateLastImportResultDisplayStatus)
	}
	// FAQ import progress route (outside of knowledge-base scope)
	faqImport := r.Group("/faq/import")
	{
		faqImport.GET("/progress/:task_id", handler.GetImportProgress)
	}
}

// RegisterKnowledgeBaseRoutes 注册知识库相关的路由
func RegisterKnowledgeBaseRoutes(r *gin.RouterGroup, handler *handler.KnowledgeBaseHandler) {
	// 知识库路由组
	kb := r.Group("/knowledge-bases")
	{
		// 创建知识库
		kb.POST("", handler.CreateKnowledgeBase)
		// 获取知识库列表
		kb.GET("", handler.ListKnowledgeBases)
		// 获取知识库详情
		kb.GET("/:id", handler.GetKnowledgeBase)
		// 更新知识库
		kb.PUT("/:id", handler.UpdateKnowledgeBase)
		// 删除知识库
		kb.DELETE("/:id", handler.DeleteKnowledgeBase)
		// 置顶/取消置顶知识库
		kb.PUT("/:id/pin", handler.TogglePinKnowledgeBase)
		// 混合搜索
		kb.GET("/:id/hybrid-search", handler.HybridSearch)
		// 拷贝知识库
		kb.POST("/copy", handler.CopyKnowledgeBase)
		// 获取知识库复制进度
		kb.GET("/copy/progress/:task_id", handler.GetKBCloneProgress)
		// 获取可移动目标知识库列表
		kb.GET("/:id/move-targets", handler.ListMoveTargets)
	}
}

// RegisterKnowledgeTagRoutes 注册知识库标签相关路由
func RegisterKnowledgeTagRoutes(r *gin.RouterGroup, tagHandler *handler.TagHandler) {
	if tagHandler == nil {
		return
	}
	kbTags := r.Group("/knowledge-bases/:id/tags")
	{
		kbTags.GET("", tagHandler.ListTags)
		kbTags.POST("", tagHandler.CreateTag)
		kbTags.PUT("/:tag_id", tagHandler.UpdateTag)
		kbTags.DELETE("/:tag_id", tagHandler.DeleteTag)
	}
}

// RegisterMessageRoutes 注册消息相关的路由
func RegisterMessageRoutes(r *gin.RouterGroup, handler *handler.MessageHandler) {
	// 消息路由组
	messages := r.Group("/messages")
	{
		// 搜索历史对话（关键词 + 向量混合搜索）
		messages.POST("/search", handler.SearchMessages)
		// 获取聊天历史知识库的统计信息
		messages.GET("/chat-history-stats", handler.GetChatHistoryKBStats)
		// 加载更早的消息，用于向上滚动加载
		messages.GET("/:session_id/load", handler.LoadMessages)
		// 删除消息
		messages.DELETE("/:session_id/:id", handler.DeleteMessage)
	}
}

// RegisterExportRoutes 注册文档导出相关的路由
func RegisterExportRoutes(r *gin.RouterGroup, h *handler.ExportHandler) {
	exportGroup := r.Group("/export")
	{
		// 导出文档（Markdown/PDF/DOCX/XLSX）
		exportGroup.POST("/document", h.ExportDocument)
		// 查询导出能力（检查后端工具是否可用）
		exportGroup.GET("/capabilities", h.ExportCapabilities)
		// Markdown → HTML 预览
		exportGroup.POST("/html", h.ExportHTML)
	}
}

// RegisterSessionRoutes 注册路由
func RegisterSessionRoutes(r *gin.RouterGroup, handler *session.Handler) {
	sessions := r.Group("/sessions")
	{
		sessions.POST("", handler.CreateSession)
		sessions.DELETE("/batch", handler.BatchDeleteSessions)
		sessions.GET("/:id", handler.GetSession)
		sessions.GET("", handler.GetSessionsByTenant)
		sessions.PUT("/:id", handler.UpdateSession)
		sessions.DELETE("/:id", handler.DeleteSession)
		sessions.DELETE("/:id/messages", handler.ClearSessionMessages)
		sessions.POST("/:session_id/generate_title", handler.GenerateTitle)
		sessions.POST("/:session_id/stop", handler.StopSession)
		// POST and DELETE share this path but gin maintains a separate radix tree
		// per HTTP verb, and the existing trees use different wildcard names
		// (POST uses :session_id, DELETE uses :id). Use whatever matches each
		// tree to avoid "wildcard conflicts" panic at route registration.
		sessions.POST("/:session_id/pin", handler.PinSession)
		sessions.DELETE("/:id/pin", handler.UnpinSession)
		// 继续接收活跃流
		sessions.GET("/continue-stream/:session_id", handler.ContinueStream)
	}
}

// RegisterChatRoutes 注册路由
func RegisterChatRoutes(r *gin.RouterGroup, handler *session.Handler) {
	knowledgeChat := r.Group("/knowledge-chat")
	{
		knowledgeChat.POST("/:session_id", handler.KnowledgeQA)
	}

	// Agent-based chat
	agentChat := r.Group("/agent-chat")
	{
		agentChat.POST("/:session_id", handler.AgentQA)
	}

	chatDocumentArtifacts := r.Group("/chat-document-artifacts")
	{
		chatDocumentArtifacts.GET("", handler.ListChatDocumentArtifacts)
		chatDocumentArtifacts.GET("/latest", handler.GetLatestChatDocumentArtifact)
		chatDocumentArtifacts.GET("/:artifact_id", handler.GetChatDocumentArtifact)
		chatDocumentArtifacts.GET("/:artifact_id/revisions", handler.ListChatDocumentArtifactRevisions)
	}

	// 新增知识检索接口，不需要session_id
	knowledgeSearch := r.Group("/knowledge-search")
	{
		knowledgeSearch.POST("", handler.SearchKnowledge)
	}
}

// RegisterTenantRoutes registers tenant routes, including tenant-member,
// invitation and audit-log subresources.
func RegisterTenantRoutes(
	r *gin.RouterGroup,
	g *rbacGuards,
	cfg *config.Config,
	handler *handler.TenantHandler,
	memberHandler *handler.TenantMemberHandler,
	invitationHandler *handler.TenantInvitationHandler,
	auditLogHandler *handler.AuditLogHandler,
) {
	// 添加获取所有租户的路由（需要跨租户权限）
	r.GET("/tenants/all", handler.ListAllTenants)
	// 添加搜索租户的路由（需要跨租户权限，支持分页和搜索）
	r.GET("/tenants/search", handler.SearchTenants)
	// 租户路由组
	tenantRoutes := r.Group("/tenants")
	{
		tenantRoutes.POST("", handler.CreateTenant)
		tenantRoutes.GET("", handler.ListTenants)

		// Tenant KV read (all authenticated users)
		tenantRoutes.GET("/kv/:key", handler.GetTenantKV)
		// Tenant KV write (all authenticated users can update their own tenant's config)
		tenantRoutes.PUT("/kv/:key", handler.UpdateTenantKV)

		tenantByID := tenantRoutes.Group("/:id", middleware.RequirePathTenantMatch(cfg))
		{
			tenantByID.GET("", handler.GetTenant)
			tenantByID.PUT("", handler.UpdateTenant)
			tenantByID.DELETE("", handler.DeleteTenant)
			tenantByID.POST("/api-key", handler.ResetAPIKey)

			if memberHandler != nil {
				tenantByID.GET("/members", middleware.RequireRole(types.TenantRoleViewer, cfg), memberHandler.ListMembers)
				tenantByID.POST("/members", middleware.RequireRole(types.TenantRoleOwner, cfg), memberHandler.AddMember)
				tenantByID.PUT("/members/:user_id", middleware.RequireRole(types.TenantRoleOwner, cfg), memberHandler.UpdateMemberRole)
				tenantByID.DELETE("/members/:user_id", middleware.RequireRole(types.TenantRoleOwner, cfg), memberHandler.RemoveMember)
				tenantByID.POST("/leave", middleware.RequireRole(types.TenantRoleViewer, cfg), memberHandler.LeaveTenant)
			}

			if invitationHandler != nil {
				tenantByID.GET("/invitations", g.Viewer(), invitationHandler.ListTenantInvitations)
				tenantByID.POST("/invitations", g.Owner(), invitationHandler.CreateInvitation)
				tenantByID.DELETE("/invitations/:inv_id", g.Owner(), invitationHandler.RevokeInvitation)
				// Share-link create lives under /invite-links so the URL
				// reads as "create a link" rather than another flavour
				// of /invitations; the underlying row still lives in the
				// tenant_invitations table and shows up in the GET above.
				tenantByID.POST("/invite-links", g.Owner(), invitationHandler.CreateInviteLink)
			}

			if auditLogHandler != nil {
				tenantByID.GET("/audit-log", middleware.RequireRole(types.TenantRoleAdmin, cfg), auditLogHandler.ListTenantAuditLog)
			}
		}
	}

	if invitationHandler != nil {
		meRoutes := r.Group("/me")
		{
			meRoutes.GET("/invitations", invitationHandler.ListMyInvitations)
			meRoutes.GET("/invitations/pending-count", invitationHandler.CountMyPendingInvitations)
			meRoutes.POST("/invitations/:inv_id/accept", invitationHandler.AcceptMyInvitation)
			meRoutes.POST("/invitations/:inv_id/decline", invitationHandler.DeclineMyInvitation)
		}
	}
}

// RegisterTenantWriteRoutes registers tenant KV write routes (super admin only)
// NOTE: This function is now unused since KV write routes have been moved to RegisterTenantRoutes
// to allow normal users to update their own tenant's configuration
func RegisterTenantWriteRoutes(r *gin.RouterGroup, handler *handler.TenantHandler) {
	// Moved to RegisterTenantRoutes
}

// RegisterModelRoutes registers model read routes (accessible to all authenticated users)
func RegisterModelRoutes(r *gin.RouterGroup, handler *handler.ModelHandler) {
	// 模型路由组 (read-only for all users)
	models := r.Group("/models")
	{
		// 获取模型厂商列表
		models.GET("/providers", handler.ListModelProviders)
		// 获取模型列表
		models.GET("", handler.ListModels)
		// 获取单个模型
		models.GET("/:id", handler.GetModel)
	}
}

// RegisterModelWriteRoutes registers model write routes (super admin only)
func RegisterModelWriteRoutes(r *gin.RouterGroup, handler *handler.ModelHandler) {
	models := r.Group("/models")
	{
		// 创建模型
		models.POST("", handler.CreateModel)
		// 更新模型
		models.PUT("/:id", handler.UpdateModel)
		// 删除模型
		models.DELETE("/:id", handler.DeleteModel)
	}
}

func RegisterEvaluationRoutes(r *gin.RouterGroup, handler *handler.EvaluationHandler) {
	evaluationRoutes := r.Group("/evaluation")
	{
		evaluationRoutes.POST("/", handler.Evaluation)
		evaluationRoutes.GET("/", handler.GetEvaluationResult)
	}
}

// RegisterAuthRoutes registers authentication routes
func RegisterAuthRoutes(r *gin.RouterGroup, handler *handler.AuthHandler) {
	r.POST("/auth/register", handler.Register)
	// Share-link surfaces are unauthenticated and accept a plaintext
	// token from the caller; rate-limit by IP to bound brute-force /
	// enumeration / abuse traffic. Limiter is shared across both
	// endpoints (see middleware/auth_public_ratelimit.go) so total
	// budget per IP is intuitive.
	publicAuthRL := middleware.PublicAuthRateLimit()
	r.POST("/auth/register-by-invite", publicAuthRL, handler.RegisterByInvite)
	r.POST("/auth/invitations/lookup", publicAuthRL, handler.LookupInvitationByToken)
	r.POST("/auth/login", handler.Login)
	r.POST("/auth/auto-setup", handler.AutoSetup)
	r.GET("/auth/oidc/config", handler.GetOIDCConfig)
	r.GET("/auth/oidc/url", handler.GetOIDCAuthorizationURL)
	r.GET("/auth/oidc/callback", handler.OIDCRedirectCallback)
	r.POST("/auth/refresh", handler.RefreshToken)
	r.GET("/auth/validate", handler.ValidateToken)
	r.POST("/auth/logout", handler.Logout)
	r.GET("/auth/me", handler.GetCurrentUser)
	r.PUT("/auth/me/preferences", handler.UpdateCurrentUserPreferences)
	r.PATCH("/auth/me/preferences", handler.UpdateCurrentUserPreferences)
	r.POST("/auth/change-password", handler.ChangePassword)
}

func RegisterInitializationRoutes(r *gin.RouterGroup, handler *handler.InitializationHandler) {
	// 初始化接口
	r.GET("/initialization/config/:kbId", handler.GetCurrentConfigByKB)
	r.POST("/initialization/initialize/:kbId", handler.InitializeByKB)
	r.PUT("/initialization/config/:kbId", handler.UpdateKBConfig) // 新的简化版接口，只传模型ID

	// Ollama相关接口
	r.GET("/initialization/ollama/status", handler.CheckOllamaStatus)
	r.GET("/initialization/ollama/models", handler.ListOllamaModels)
	r.POST("/initialization/ollama/models/check", handler.CheckOllamaModels)
	r.POST("/initialization/ollama/models/download", handler.DownloadOllamaModel)
	r.GET("/initialization/ollama/download/progress/:taskId", handler.GetDownloadProgress)
	r.GET("/initialization/ollama/download/tasks", handler.ListDownloadTasks)

	// 远程API相关接口
	r.POST("/initialization/remote/check", handler.CheckRemoteModel)
	r.POST("/initialization/embedding/test", handler.TestEmbeddingModel)
	r.POST("/initialization/rerank/check", handler.CheckRerankModel)
	r.POST("/initialization/asr/check", handler.CheckASRModel)
	r.POST("/initialization/multimodal/test", handler.TestMultimodalFunction)

	r.POST("/initialization/extract/text-relation", handler.ExtractTextRelations)
	r.POST("/initialization/extract/fabri-tag", handler.FabriTag)
	r.POST("/initialization/extract/fabri-text", handler.FabriText)
}

// RegisterSystemRoutes registers system information routes
func RegisterSystemRoutes(r *gin.RouterGroup, handler *handler.SystemHandler) {
	systemRoutes := r.Group("/system")
	{
		systemRoutes.GET("/info", handler.GetSystemInfo)
		systemRoutes.GET("/parser-engines", handler.ListParserEngines)
		systemRoutes.POST("/parser-engines/check", handler.CheckParserEngines)
		systemRoutes.POST("/docreader/reconnect", handler.ReconnectDocReader)
		systemRoutes.GET("/storage-engine-status", handler.GetStorageEngineStatus)
		systemRoutes.POST("/storage-engine-check", handler.CheckStorageEngine)
	}
}

// RegisterMCPServiceRoutes registers MCP service routes
func RegisterMCPServiceRoutes(r *gin.RouterGroup, handler *handler.MCPServiceHandler) {
	mcpServices := r.Group("/mcp-services")
	{
		// Create MCP service
		mcpServices.POST("", handler.CreateMCPService)
		// List MCP services
		mcpServices.GET("", handler.ListMCPServices)
		// Get MCP service by ID
		mcpServices.GET("/:id", handler.GetMCPService)
		// Update MCP service
		mcpServices.PUT("/:id", handler.UpdateMCPService)
		// Delete MCP service
		mcpServices.DELETE("/:id", handler.DeleteMCPService)
		// Test MCP service connection
		mcpServices.POST("/:id/test", handler.TestMCPService)
		// Get MCP service tools
		mcpServices.GET("/:id/tools", handler.GetMCPServiceTools)
		// Get MCP service resources
		mcpServices.GET("/:id/resources", handler.GetMCPServiceResources)
		// MCP tool human approval (issue #1173)
		mcpServices.GET("/:id/tool-approvals", handler.ListMCPToolApprovals)
		mcpServices.PUT("/:id/tool-approvals/:tool_name", handler.SetMCPToolApproval)
	}

	agentTool := r.Group("/agent")
	{
		agentTool.POST("/tool-approvals/:pending_id", handler.ResolveToolApproval)
	}
}

// RegisterWebSearchRoutes registers web search routes
func RegisterWebSearchRoutes(r *gin.RouterGroup, webSearchHandler *handler.WebSearchHandler) {
	// Web search providers
	webSearch := r.Group("/web-search")
	{
		// Get available providers
		webSearch.GET("/providers", webSearchHandler.GetProviders)
	}
}

// RegisterWebSearchProviderRoutes registers CRUD routes for web search provider configurations
func RegisterWebSearchProviderRoutes(r *gin.RouterGroup, h *handler.WebSearchProviderHandler) {
	providers := r.Group("/web-search-providers")
	{
		// List available provider types (metadata for UI forms)
		providers.GET("/types", h.ListProviderTypes)
		// Test with raw credentials (no persistence)
		providers.POST("/test", h.TestProviderRaw)
		// CRUD
		providers.POST("", h.CreateProvider)
		providers.GET("", h.ListProviders)
		providers.GET("/:id", h.GetProvider)
		providers.PUT("/:id", h.UpdateProvider)
		providers.DELETE("/:id", h.DeleteProvider)
		// Test existing saved provider
		providers.POST("/:id/test", h.TestProviderByID)
	}
}

// RegisterVectorStoreRoutes registers CRUD routes for vector store configurations
func RegisterVectorStoreRoutes(r *gin.RouterGroup, h *handler.VectorStoreHandler) {
	stores := r.Group("/vector-stores")
	{
		// List available engine types (metadata for UI forms)
		stores.GET("/types", h.ListStoreTypes)
		// Test with raw credentials (no persistence)
		stores.POST("/test", h.TestStoreRaw)
		// CRUD
		stores.POST("", h.CreateStore)
		stores.GET("", h.ListStores)
		stores.GET("/:id", h.GetStore)
		stores.PUT("/:id", h.UpdateStore)
		stores.DELETE("/:id", h.DeleteStore)
		// Test existing saved or env store
		stores.POST("/:id/test", h.TestStoreByID)
	}
}

// RegisterCustomAgentRoutes registers custom agent routes
func RegisterCustomAgentRoutes(r *gin.RouterGroup, agentHandler *handler.CustomAgentHandler) {
	agents := r.Group("/agents")
	{
		// Get placeholder definitions (must be before /:id to avoid conflict)
		agents.GET("/placeholders", agentHandler.GetPlaceholders)
		// List smart-reasoning agent type presets (rag-qa / wiki-qa / hybrid / custom)
		agents.GET("/type-presets", agentHandler.GetAgentTypePresets)
		// Create custom agent
		agents.POST("", agentHandler.CreateAgent)
		// List all agents (including built-in)
		agents.GET("", agentHandler.ListAgents)
		// Get the current page-share state of one custom agent
		agents.GET("/:id/page-share", agentHandler.GetAgentPageShare)
		// Open or re-enable page share for one custom agent
		agents.POST("/:id/page-share", agentHandler.CreateOrEnableAgentPageShare)
		// Close page share for one custom agent
		agents.DELETE("/:id/page-share", agentHandler.DeleteAgentPageShare)
		// Get agent by ID
		agents.GET("/:id", agentHandler.GetAgent)
		// Update agent
		agents.PUT("/:id", agentHandler.UpdateAgent)
		// Delete agent
		agents.DELETE("/:id", agentHandler.DeleteAgent)
		// Copy agent
		agents.POST("/:id/copy", agentHandler.CopyAgent)
	}
	// Registered outside the group to avoid Gin route conflict with /agents/:id/shares in organization routes
	r.GET("/agents/:id/suggested-questions", agentHandler.GetSuggestedQuestions)
}

// RegisterPublicAgentPageShareRoutes registers anonymous readonly routes for agent page shares.
func RegisterPublicAgentPageShareRoutes(r *gin.Engine, agentHandler *handler.CustomAgentHandler) {
	public := r.Group("/api/v1/public")
	{
		public.GET("/agent-page-shares/:share_code", agentHandler.GetPublicAgentPageShare)
	}
}

// RegisterPublicAgentPageShareChatRoutes registers anonymous session and chat routes for agent share pages.
func RegisterPublicAgentPageShareChatRoutes(r *gin.Engine, sessionHandler *session.Handler) {
	public := r.Group("/api/v1/public")
	{
		public.POST("/agent-page-shares/:share_code/sessions", sessionHandler.CreatePublicAgentPageShareSession)
		public.GET("/agent-page-shares/:share_code/sessions/:session_id/messages", sessionHandler.LoadPublicAgentPageShareMessages)
		public.POST("/agent-page-shares/:share_code/chat", sessionHandler.PublicAgentPageShareChat)
		public.GET("/agent-page-shares/:share_code/chat/continue", sessionHandler.ContinuePublicAgentPageShareStream)
		public.POST("/agent-page-shares/:share_code/chat/continue", sessionHandler.ContinuePublicAgentPageShareStream)
		public.GET("/agent-page-shares/:share_code/sessions/continue-stream/:session_id", sessionHandler.ContinuePublicAgentPageShareStream)
		public.POST("/agent-page-shares/:share_code/sessions/:session_id/stop", sessionHandler.StopPublicAgentPageShareSession)
	}
}

// RegisterPublicAgentPageShareExportRoutes registers anonymous export routes for agent share pages.
func RegisterPublicAgentPageShareExportRoutes(r *gin.Engine, exportHandler *handler.ExportHandler) {
	public := r.Group("/api/v1/public")
	{
		public.GET("/agent-page-shares/:share_code/export/capabilities", exportHandler.PublicAgentPageShareExportCapabilities)
		public.POST("/agent-page-shares/:share_code/export/document", exportHandler.PublicAgentPageShareExportDocument)
	}
}

// RegisterSkillRoutes registers skill routes
func RegisterSkillRoutes(r *gin.RouterGroup, skillHandler *handler.SkillHandler) {
	skills := r.Group("/skills")
	{
		// List all preloaded skills
		skills.GET("", skillHandler.ListSkills)
	}
}

// RegisterOrganizationRoutes registers organization and sharing routes
func RegisterOrganizationRoutes(r *gin.RouterGroup, orgHandler *handler.OrganizationHandler) {
	// Organization routes
	orgs := r.Group("/organizations")
	{
		// Create organization
		orgs.POST("", orgHandler.CreateOrganization)
		// List my organizations
		orgs.GET("", orgHandler.ListMyOrganizations)
		// Preview organization by invite code (without joining)
		orgs.GET("/preview/:code", orgHandler.PreviewByInviteCode)
		// Join organization by invite code
		orgs.POST("/join", orgHandler.JoinByInviteCode)
		// Submit join request (for organizations that require approval)
		orgs.POST("/join-request", orgHandler.SubmitJoinRequest)
		// Search searchable (discoverable) organizations
		orgs.GET("/search", orgHandler.SearchOrganizations)
		// Join searchable organization by ID (no invite code)
		orgs.POST("/join-by-id", orgHandler.JoinByOrganizationID)
		// Get organization by ID
		orgs.GET("/:id", orgHandler.GetOrganization)
		// Update organization
		orgs.PUT("/:id", orgHandler.UpdateOrganization)
		// Delete organization
		orgs.DELETE("/:id", orgHandler.DeleteOrganization)
		// Leave organization
		orgs.POST("/:id/leave", orgHandler.LeaveOrganization)
		// Request role upgrade (for existing members)
		orgs.POST("/:id/request-upgrade", orgHandler.RequestRoleUpgrade)
		// Generate invite code
		orgs.POST("/:id/invite-code", orgHandler.GenerateInviteCode)
		// Search users for invite (admin only)
		orgs.GET("/:id/search-users", orgHandler.SearchUsersForInvite)
		// Invite member directly (admin only)
		orgs.POST("/:id/invite", orgHandler.InviteMember)
		// List members
		orgs.GET("/:id/members", orgHandler.ListMembers)
		// Update member role
		orgs.PUT("/:id/members/:user_id", orgHandler.UpdateMemberRole)
		// Remove member
		orgs.DELETE("/:id/members/:user_id", orgHandler.RemoveMember)
		// List join requests (admin only)
		orgs.GET("/:id/join-requests", orgHandler.ListJoinRequests)
		// Review join request (admin only)
		orgs.PUT("/:id/join-requests/:request_id/review", orgHandler.ReviewJoinRequest)
		// List knowledge bases shared to this organization
		orgs.GET("/:id/shares", orgHandler.ListOrgShares)
		// List agents shared to this organization
		orgs.GET("/:id/agent-shares", orgHandler.ListOrgAgentShares)
		// List all knowledge bases in this organization (including mine) for list-page space view
		orgs.GET("/:id/shared-knowledge-bases", orgHandler.ListOrganizationSharedKnowledgeBases)
		// List all agents in this organization (including mine) for list-page space view
		orgs.GET("/:id/shared-agents", orgHandler.ListOrganizationSharedAgents)
	}

	// Knowledge base sharing routes (add to existing kb routes)
	kbShares := r.Group("/knowledge-bases/:id/shares")
	{
		// Share knowledge base
		kbShares.POST("", orgHandler.ShareKnowledgeBase)
		// List shares
		kbShares.GET("", orgHandler.ListKBShares)
		// Update share permission
		kbShares.PUT("/:share_id", orgHandler.UpdateSharePermission)
		// Remove share
		kbShares.DELETE("/:share_id", orgHandler.RemoveShare)
	}

	// Agent sharing routes
	agentShares := r.Group("/agents/:id/shares")
	{
		agentShares.POST("", orgHandler.ShareAgent)
		agentShares.GET("", orgHandler.ListAgentShares)
		agentShares.DELETE("/:share_id", orgHandler.RemoveAgentShare)
	}

	// Shared knowledge bases route
	r.GET("/shared-knowledge-bases", orgHandler.ListSharedKnowledgeBases)
	// Shared agents route
	r.GET("/shared-agents", orgHandler.ListSharedAgents)
	r.POST("/shared-agents/disabled", orgHandler.SetSharedAgentDisabledByMe)
}

// RegisterIMRoutes registers IM callback routes.
// These are registered BEFORE auth middleware since IM platforms use their own signature verification.
func RegisterIMRoutes(r *gin.Engine, imHandler *handler.IMHandler) {
	im := r.Group("/api/v1/im")
	{
		im.GET("/callback/:channel_id", imHandler.IMCallback)
		im.POST("/callback/:channel_id", imHandler.IMCallback)
	}
}

// RegisterIMChannelRoutes registers IM channel CRUD routes (requires authentication).
func RegisterIMChannelRoutes(r *gin.RouterGroup, imHandler *handler.IMHandler) {
	// Channel CRUD under agents
	agentChannels := r.Group("/agents/:id/im-channels")
	{
		agentChannels.POST("", imHandler.CreateIMChannel)
		agentChannels.GET("", imHandler.ListIMChannels)
	}

	// Channel operations by channel ID
	channels := r.Group("/im-channels")
	{
		channels.GET("", imHandler.ListAllIMChannels)
		channels.PUT("/:id", imHandler.UpdateIMChannel)
		channels.DELETE("/:id", imHandler.DeleteIMChannel)
		channels.POST("/:id/toggle", imHandler.ToggleIMChannel)
	}

	// WeChat QR code login (requires authentication)
	wechatGroup := r.Group("/wechat")
	{
		wechatGroup.POST("/qrcode", imHandler.WeChatGetQRCode)
		wechatGroup.POST("/qrcode/status", imHandler.WeChatPollQRCodeStatus)
	}
}

// serveFrontendStatic registers a middleware that serves the frontend SPA
// from the ./web directory if it exists. Must be called BEFORE auth middleware
// so static files are served without authentication.
func serveFrontendStatic(r *gin.Engine) {
	webDir := os.Getenv("WEKNORA_WEB_DIR")
	if webDir == "" {
		webDir = "./web"
	}
	absDir, _ := filepath.Abs(webDir)
	indexPath := filepath.Join(absDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}

	logger.Infof(context.Background(), "[Router] Serving frontend static files from %s", absDir)

	fs := http.Dir(absDir)
	fileServer := http.FileServer(fs)

	r.Use(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/health") || strings.HasPrefix(path, "/swagger/") {
			c.Next()
			return
		}
		fullPath := filepath.Join(absDir, path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			setFrontendCacheHeaders(c.Writer, path)
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}
		setFrontendCacheHeaders(c.Writer, "/index.html")
		c.File(indexPath)
		c.Abort()
	})
}

// setFrontendCacheHeaders sets Cache-Control headers for frontend static resources.
// Vite 构建产物中 /assets/* 的文件名带 hash，可长期缓存；其余（index.html、config.js、favicon 等）
// 每次都需 revalidate，避免前端升级后用户看到旧版本。
func setFrontendCacheHeaders(w http.ResponseWriter, path string) {
	if strings.HasPrefix(path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
}

// serveFiles serves files via query parameters and tenant storage settings.
// It is registered after auth middleware, so tenant context comes from authentication.
//
// Route:
//   - /files?file_path=<provider://...>
type getRouteRegistrar interface {
	GET(string, ...gin.HandlerFunc) gin.IRoutes
}

func serveFiles(r getRouteRegistrar, globalFileService interfaces.FileService) {
	baseDir := os.Getenv("LOCAL_STORAGE_BASE_DIR")
	if baseDir == "" {
		baseDir = "/data/files"
	}
	absDir, _ := filepath.Abs(baseDir)
	if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
		if err := os.MkdirAll(absDir, 0o755); err != nil {
			logger.Warnf(context.Background(), "[Router] Cannot create local storage dir %s: %v", absDir, err)
		}
	}

	logger.Infof(context.Background(), "[Router] Serving files from /files (local base: %s)", absDir)

	r.GET("/files", func(c *gin.Context) {
		filePath := strings.TrimSpace(c.Query("file_path"))
		if filePath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing required parameter: file_path"})
			return
		}
		if strings.Contains(filePath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file path"})
			return
		}

		provider := types.ParseProviderScheme(filePath)

		tenant, _ := c.Request.Context().Value(types.TenantInfoContextKey).(*types.Tenant)
		if tenant == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: tenant context missing"})
			return
		}

		if err := secutils.ValidateStoragePathTenant(filePath, tenant.ID); err != nil {
			logger.Warnf(context.Background(), "[Router] /files denied cross-tenant or invalid path: tenant_id=%d file_path=%q err=%v", tenant.ID, filePath, err)
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: file path not accessible"})
			return
		}

		var (
			fileSvc          interfaces.FileService
			resolvedProvider string
			err              error
		)

		if tenant.StorageEngineConfig != nil {
			fileSvc, resolvedProvider, err = filesvc.NewFileServiceFromStorageConfig(provider, tenant.StorageEngineConfig, absDir)
		} else {
			err = http.ErrMissingFile
		}
		if err != nil {
			globalStorageType := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_TYPE")))
			if globalStorageType == "" {
				globalStorageType = "local"
			}
			if provider == globalStorageType && globalFileService != nil {
				logger.Warnf(context.Background(), "[Router] /files tenant storage config missing or invalid, fallback to global file service: tenant_id=%d provider=%s err=%v", tenant.ID, provider, err)
				fileSvc = globalFileService
				resolvedProvider = globalStorageType
			} else {
				logger.Warnf(context.Background(), "[Router] /files resolve file service failed without fallback: tenant_id=%d provider=%s global_storage_type=%s err=%v", tenant.ID, provider, globalStorageType, err)
				c.Status(http.StatusBadRequest)
				return
			}
		}

		reader, err := fileSvc.GetFile(c.Request.Context(), filePath)
		if err != nil {
			logger.Warnf(context.Background(), "[Router] /files get file failed: tenant_id=%d provider=%s path=%q err=%v", tenant.ID, resolvedProvider, filePath, err)
			c.Status(http.StatusNotFound)
			return
		}
		defer reader.Close()

		ext := filepath.Ext(filePath)
		contentType := "application/octet-stream"
		switch strings.ToLower(ext) {
		case ".png":
			contentType = "image/png"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".gif":
			contentType = "image/gif"
		case ".webp":
			contentType = "image/webp"
		case ".bmp":
			contentType = "image/bmp"
		case ".svg":
			contentType = "image/svg+xml"
		case ".pdf":
			contentType = "application/pdf"
		case ".csv":
			contentType = "text/csv; charset=utf-8"
		}

		c.Header("Content-Type", contentType)
		c.Header("Cache-Control", "public, max-age=86400")
		c.Status(http.StatusOK)
		if _, err := io.Copy(c.Writer, reader); err != nil {
			logger.Warnf(context.Background(), "[Router] /files write response failed: %v", err)
		}
	})
}

// servePresignedFiles serves files via HMAC-signed URLs without requiring authentication.
// This is used by IM channels to serve images that are embedded in bot replies.
//
// Routes:
//   - GET  /api/v1/files/presigned?file_path=<provider://...>&tenant_id=<id>&expires=<unix>&sig=<hmac>
//   - HEAD /api/v1/files/presigned?...  (IM platforms issue HEAD first to validate
//     Content-Type / Content-Length before rendering image previews; HEAD must
//     succeed or the inline image renders as broken)
//
// Failure paths log client IP + User-Agent + (truncated) file_path so operators
// can correlate an IM platform's fetch against the upstream signing log line.
// Without this it is otherwise impossible to tell whether a "broken image" is
// caused by an expired signature, a stale URL cached by the platform, the
// platform's IP being blocked, or the URL simply never reaching us.
func servePresignedFiles(r *gin.Engine, tenantService interfaces.TenantService) {
	baseDir := os.Getenv("LOCAL_STORAGE_BASE_DIR")
	if baseDir == "" {
		baseDir = "/data/files"
	}
	absDir, _ := filepath.Abs(baseDir)

	handler := presignedFileHandler(tenantService, absDir)
	r.GET("/api/v1/files/presigned", handler)
	r.HEAD("/api/v1/files/presigned", handler)
}

// presignedFileHandler returns the shared Gin handler used by both GET and HEAD.
// For HEAD requests it returns the same status + headers but does not stream
// the body — this is enough for IM platforms to validate the URL while saving
// us a full read of the backing object.
func presignedFileHandler(tenantService interfaces.TenantService, absDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()

		filePath := strings.TrimSpace(c.Query("file_path"))
		tenantIDStr := strings.TrimSpace(c.Query("tenant_id"))
		expiresStr := strings.TrimSpace(c.Query("expires"))
		sig := strings.TrimSpace(c.Query("sig"))

		if filePath == "" || tenantIDStr == "" || expiresStr == "" || sig == "" {
			logger.Warnf(ctx, "[Router] /files/presigned missing params: client_ip=%s ua=%q file_path=%q tenant_id=%q expires=%q has_sig=%v",
				clientIP, userAgent, filePath, tenantIDStr, expiresStr, sig != "")
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing required parameters"})
			return
		}
		if strings.Contains(filePath, "..") {
			logger.Warnf(ctx, "[Router] /files/presigned rejected path traversal: client_ip=%s file_path=%q", clientIP, filePath)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file path"})
			return
		}

		tenantID, err := strconv.ParseUint(tenantIDStr, 10, 64)
		if err != nil {
			logger.Warnf(ctx, "[Router] /files/presigned invalid tenant_id: client_ip=%s tenant_id=%q err=%v", clientIP, tenantIDStr, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tenant_id"})
			return
		}

		// Verify HMAC signature and expiry. Logged at Warn because every 403
		// here is a signal worth investigating: either the URL was tampered
		// with, the IM platform cached an expired URL, or SYSTEM_AES_KEY was
		// rotated without invalidating in-flight links.
		if !secutils.VerifyFileURLSig(filePath, tenantID, expiresStr, sig) {
			logger.Warnf(ctx, "[Router] /files/presigned sig invalid or expired: client_ip=%s ua=%q tenant_id=%d file_path=%q expires=%s",
				clientIP, userAgent, tenantID, filePath, expiresStr)
			c.JSON(http.StatusForbidden, gin.H{"error": "invalid or expired signature"})
			return
		}

		provider := types.ParseProviderScheme(filePath)
		tenant, err := tenantService.GetTenantByID(ctx, tenantID)
		if err != nil {
			logger.Warnf(ctx, "[Router] /files/presigned tenant lookup failed: client_ip=%s tenant_id=%d err=%v", clientIP, tenantID, err)
			c.Status(http.StatusNotFound)
			return
		}

		fileSvc, resolvedProvider, err := filesvc.NewFileServiceFromStorageConfig(provider, tenant.StorageEngineConfig, absDir)
		if err != nil {
			logger.Warnf(ctx, "[Router] /files/presigned resolve file service failed: client_ip=%s tenant_id=%d provider=%s err=%v",
				clientIP, tenantID, provider, err)
			c.Status(http.StatusBadRequest)
			return
		}

		ext := filepath.Ext(filePath)
		contentType := "application/octet-stream"
		switch strings.ToLower(ext) {
		case ".png":
			contentType = "image/png"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".gif":
			contentType = "image/gif"
		case ".webp":
			contentType = "image/webp"
		case ".bmp":
			contentType = "image/bmp"
		case ".svg":
			contentType = "image/svg+xml"
		case ".pdf":
			contentType = "application/pdf"
		}

		// HEAD short-circuits the body read. We still need to confirm the
		// object exists, but we use a 0-byte content length and skip io.Copy.
		// Skipping GetFile entirely for HEAD would risk reporting 200 for a
		// signed URL that no longer points at a real object; that mismatch
		// would make subsequent GETs from the same client mysteriously fail.
		reader, err := fileSvc.GetFile(ctx, filePath)
		if err != nil {
			logger.Warnf(ctx, "[Router] /files/presigned get file failed: client_ip=%s tenant_id=%d provider=%s path=%q err=%v",
				clientIP, tenantID, resolvedProvider, filePath, err)
			c.Status(http.StatusNotFound)
			return
		}
		defer reader.Close()

		c.Header("Content-Type", contentType)
		c.Header("Cache-Control", "public, max-age=86400")
		if c.Request.Method == http.MethodHead {
			c.Status(http.StatusOK)
			return
		}
		c.Status(http.StatusOK)
		if _, err := io.Copy(c.Writer, reader); err != nil {
			logger.Warnf(ctx, "[Router] /files/presigned write response failed: client_ip=%s tenant_id=%d err=%v", clientIP, tenantID, err)
		}
	}
}

// servePresignedPreview registers an Admin-only diagnostic endpoint that
// returns the presigned HTTP URL that *would be* generated for a given
// storage path by the calling tenant's current storage config — exactly the
// URL an IM channel would embed in a reply. Operators can paste the result
// into a 4G/mobile browser to verify public reachability without having to
// send a real message through an IM bot.
//
// Route:
//   - GET /api/v1/files/presigned-preview?file_path=<provider://...>
func servePresignedPreview(r *gin.Engine, cfg *config.Config) {
	baseDir := os.Getenv("LOCAL_STORAGE_BASE_DIR")
	if baseDir == "" {
		baseDir = "/data/files"
	}
	absDir, _ := filepath.Abs(baseDir)

	r.GET("/api/v1/files/presigned-preview",
		middleware.RequireRole(types.TenantRoleAdmin, cfg),
		func(c *gin.Context) {
			ctx := c.Request.Context()
			filePath := strings.TrimSpace(c.Query("file_path"))
			if filePath == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "missing required parameter: file_path"})
				return
			}
			if strings.Contains(filePath, "..") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file path"})
				return
			}

			tenant, _ := ctx.Value(types.TenantInfoContextKey).(*types.Tenant)
			if tenant == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: tenant context missing"})
				return
			}

			provider := types.ParseProviderScheme(filePath)
			fileSvc, resolvedProvider, err := filesvc.NewFileServiceFromStorageConfig(provider, tenant.StorageEngineConfig, absDir)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":    err.Error(),
					"provider": provider,
					"hint":     "tenant storage config is missing or incomplete for this provider",
				})
				return
			}

			httpURL, err := fileSvc.GetFileURL(ctx, filePath)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":    err.Error(),
					"provider": resolvedProvider,
					"hint":     "GetFileURL failed; for local storage this usually means APP_EXTERNAL_URL is unset",
				})
				return
			}

			// Detect the "no-op" case where local storage falls back to the
			// provider:// path because APP_EXTERNAL_URL is missing. Surfacing
			// this explicitly is the whole point of the endpoint.
			rewritten := httpURL != filePath
			hint := ""
			if !rewritten {
				hint = "URL unchanged; for local storage set APP_EXTERNAL_URL to enable presigned HTTP URLs"
			}

			c.JSON(http.StatusOK, gin.H{
				"file_path": filePath,
				"provider":  resolvedProvider,
				"url":       httpURL,
				"rewritten": rewritten,
				"hint":      hint,
			})
		})
}

// RegisterDataSourceRoutes 注册数据源相关的路由
func RegisterDataSourceRoutes(r *gin.RouterGroup, handler *handler.DataSourceHandler) {
	r.GET("/knowledge-bases/:id/database-schema", handler.GetDatabaseSchema)
	r.GET("/database-query-audits", handler.ListDatabaseQueryAudits)

	// Data source routes
	ds := r.Group("/datasource")
	{
		// Get available connector types
		ds.GET("/types", handler.GetAvailableConnectors)

		// Validate credentials without persistence (for "Test Connection" button)
		ds.POST("/validate-credentials", handler.ValidateCredentials)

		// CRUD operations
		ds.POST("", handler.CreateDataSource)
		ds.GET("", handler.ListDataSources)
		ds.GET("/:id", handler.GetDataSource)
		ds.PUT("/:id", handler.UpdateDataSource)
		ds.DELETE("/:id", handler.DeleteDataSource)

		// Connection and resource management
		ds.POST("/:id/validate", handler.ValidateConnection)
		ds.POST("/:id/refresh-schema", handler.RefreshSchema)
		ds.GET("/:id/resources", handler.ListAvailableResources)

		// Sync management
		ds.POST("/:id/sync", handler.ManualSync)
		ds.POST("/:id/pause", handler.PauseDataSource)
		ds.POST("/:id/resume", handler.ResumeDataSource)

		// Sync logs
		ds.GET("/:id/logs", handler.GetSyncLogs)
		ds.GET("/logs/:log_id", handler.GetSyncLog)
	}
}

// RegisterWeKnoraCloudRoutes 注册 WeKnoraCloud 初始化路由
func RegisterWeKnoraCloudRoutes(r *gin.RouterGroup, handler *handler.WeKnoraCloudHandler) {
	r.POST("/weknoracloud/credentials", handler.SaveCredentials)
	r.GET("/models/weknoracloud/status", handler.Status)
}

// RegisterWikiPageRoutes registers wiki page related routes
func RegisterWikiPageRoutes(r *gin.RouterGroup, wikiHandler *handler.WikiPageHandler) {
	wiki := r.Group("/knowledgebase/:kb_id/wiki")
	{
		// Page CRUD
		wiki.GET("/pages", wikiHandler.ListPages)
		wiki.POST("/pages", wikiHandler.CreatePage)
		wiki.GET("/pages/*slug", wikiHandler.GetPage)
		wiki.PUT("/pages/*slug", wikiHandler.UpdatePage)
		wiki.DELETE("/pages/*slug", wikiHandler.DeletePage)

		// Special pages
		wiki.GET("/index", wikiHandler.GetIndex)
		wiki.GET("/log", wikiHandler.GetLog)

		// Search and maintenance
		wiki.GET("/search", wikiHandler.SearchPages)
		wiki.POST("/rebuild-links", wikiHandler.RebuildLinks)
		wiki.GET("/lint", wikiHandler.Lint)
		wiki.POST("/auto-fix", wikiHandler.AutoFix)

		// Issues
		wiki.GET("/issues", wikiHandler.ListIssues)
		wiki.PUT("/issues/:issue_id/status", wikiHandler.UpdateIssueStatus)
	}
}

// RegisterOrgTreeRoutes registers organization tree management routes.
// Fine-grained permission checks are enforced in each handler.
func RegisterOrgTreeRoutes(r *gin.RouterGroup, orgTreeHandler *handler.OrgTreeHandler) {
	orgTree := r.Group("/org-tree")
	{
		// Search users for assignment (must be before /:id to avoid conflict)
		orgTree.GET("/search-users", orgTreeHandler.SearchUsersForAssign)
		// Get the full organization tree (org admins see only their subtrees)
		orgTree.GET("", orgTreeHandler.GetOrgTree)
		orgTree.POST("", orgTreeHandler.CreateOrgNode)
		orgTree.GET("/:id", orgTreeHandler.GetOrgNode)
		orgTree.PUT("/:id", orgTreeHandler.UpdateOrgNode)
		orgTree.DELETE("/:id", orgTreeHandler.DeleteOrgNode)
		orgTree.POST("/:id/move", orgTreeHandler.MoveOrgNode)
		orgTree.GET("/:id/members", orgTreeHandler.ListOrgMembers)
		orgTree.POST("/:id/members", orgTreeHandler.AssignUser)
		orgTree.POST("/:id/create-user", orgTreeHandler.CreateUserInOrg)
		orgTree.PUT("/:id/users/:user_id", orgTreeHandler.UpdateUserInOrg)
		orgTree.DELETE("/:id/members/:user_id", orgTreeHandler.RemoveUser)
		orgTree.PUT("/:id/admin", orgTreeHandler.SetOrgAdmin)
	}
}

// RegisterOrgTreeSuperAdminRoutes registers org-tree routes that require super admin privileges.
func RegisterOrgTreeSuperAdminRoutes(r *gin.RouterGroup, orgTreeHandler *handler.OrgTreeHandler) {
	r.PUT("/org-tree/super-admin", orgTreeHandler.SetSuperAdmin)
	r.PUT("/org-tree/:id/users/:user_id/password", orgTreeHandler.UpdateUserPasswordInOrg)
}
