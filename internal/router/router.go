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
	"github.com/redis/go-redis/v9"
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

	Config                       *config.Config
	FileService                  interfaces.FileService
	UserService                  interfaces.UserService
	KBService                    interfaces.KnowledgeBaseService
	KnowledgeService             interfaces.KnowledgeService
	ChunkService                 interfaces.ChunkService
	SessionService               interfaces.SessionService
	MessageService               interfaces.MessageService
	ModelService                 interfaces.ModelService
	EvaluationService            interfaces.EvaluationService
	KBShareService               interfaces.KBShareService
	AgentShareService            interfaces.AgentShareService
	KBHandler                    *handler.KnowledgeBaseHandler
	KnowledgeHandler             *handler.KnowledgeHandler
	TenantHandler                *handler.TenantHandler
	TenantService                interfaces.TenantService
	TenantMemberService          interfaces.TenantMemberService
	TenantMemberHandler          *handler.TenantMemberHandler
	TenantInvitationHandler      *handler.TenantInvitationHandler
	AuditLogHandler              *handler.AuditLogHandler
	AuditLogService              interfaces.AuditLogService
	ChunkHandler                 *handler.ChunkHandler
	SessionHandler               *session.Handler
	MessageHandler               *handler.MessageHandler
	ModelHandler                 *handler.ModelHandler
	ModelCredentialsHandler      *handler.ModelCredentialsHandler
	EvaluationHandler            *handler.EvaluationHandler
	AuthHandler                  *handler.AuthHandler
	InitializationHandler        *handler.InitializationHandler
	SystemHandler                *handler.SystemHandler
	MCPServiceHandler            *handler.MCPServiceHandler
	MCPCredentialsHandler        *handler.MCPCredentialsHandler
	MCPOAuthHandler              *handler.MCPOAuthHandler
	WebSearchHandler             *handler.WebSearchHandler
	WebSearchProviderHandler     *handler.WebSearchProviderHandler
	WebSearchCredentialsHandler  *handler.WebSearchProviderCredentialsHandler
	VectorStoreHandler           *handler.VectorStoreHandler
	FAQHandler                   *handler.FAQHandler
	TagHandler                   *handler.TagHandler
	CustomAgentHandler           *handler.CustomAgentHandler
	UserFavoriteHandler          *handler.UserResourceFavoriteHandler
	SkillHandler                 *handler.SkillHandler
	OrganizationHandler          *handler.OrganizationHandler
	IMHandler                    *handler.IMHandler
	EmbedChannelHandler          *handler.EmbedChannelHandler
	EmbedChannelService          interfaces.EmbedChannelService
	RedisClient                  *redis.Client
	DataSourceHandler            *handler.DataSourceHandler
	DataSourceCredentialsHandler *handler.DataSourceCredentialsHandler
	WeKnoraCloudHandler          *handler.WeKnoraCloudHandler
	WikiPageHandler              *handler.WikiPageHandler
	OrgTreeHandler               *handler.OrgTreeHandler
	ExportHandler                *handler.ExportHandler
}

// NewRouter 创建新的路由
func NewRouter(params RouterParams) *gin.Engine {
	r := gin.New()
	r.ContextWithFallback = true

	// Trusted proxies: gin defaults to trusting ALL proxies, which makes
	// c.ClientIP() honor a client-supplied X-Forwarded-For. Public, unauthed
	// embed endpoints rate-limit per (channel, ClientIP), so a spoofed XFF would
	// trivially bypass the limiter. Restrict to the fronting proxy network so
	// only the real client IP (appended by nginx) is returned. Configurable via
	// WEKNORA_TRUSTED_PROXIES (comma-separated CIDRs/IPs).
	if err := r.SetTrustedProxies(trustedProxies()); err != nil {
		logger.Errorf(context.Background(), "[Router] failed to set trusted proxies: %v", err)
	}

	// CORS 中间件应放在最前面
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key", "X-Request-ID", "X-Share-Session-Token", "X-Tenant-ID", "X-Embed-Session"},
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

	// Embed page framing policy: emit a per-channel `frame-ancestors` CSP so the
	// embed SPA page (/embed/:channelId) can only be iframed by the channel's
	// allowed origins. This is the page-level counterpart to the API Origin
	// allowlist enforced in EmbedAuth. Registered before the static handler so
	// it runs for the embed HTML response.
	if params.EmbedChannelService != nil {
		r.Use(embedFrameAncestorsMiddleware(params.EmbedChannelService))
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

	// Web embed 公开路由（使用 publish token 鉴权，不走全局 Auth）
	RegisterEmbedPublicRoutes(r, params.EmbedChannelHandler, params.EmbedChannelService, params.TenantService, params.RedisClient, params.FileService)

	// 认证中间件
	r.Use(middleware.Auth(params.TenantService, params.UserService, params.TenantMemberService, params.Config))

	// 文件服务：统一代理本地/MinIO/COS/TOS存储后端（需要认证）
	serveFiles(r, params.FileService)

	// Presigned file access: no auth required, signature-verified.
	servePresignedFiles(r, params.TenantService)

	// Diagnostic preview of presigned URLs (Admin only, behind auth middleware).
	servePresignedPreview(r, params.Config)

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
		RegisterTenantRoutes(v1, params.TenantHandler, params.TenantMemberHandler, params.TenantInvitationHandler, params.AuditLogHandler, g)
		RegisterMyInvitationRoutes(v1, params.TenantInvitationHandler)
		RegisterKnowledgeBaseRoutes(v1, params.KBHandler, g)
		RegisterKnowledgeTagRoutes(v1, params.TagHandler, g)
		RegisterKnowledgeRoutes(v1, params.KnowledgeHandler, g)
		RegisterFAQRoutes(v1, params.FAQHandler, g)
		RegisterChunkRoutes(v1, params.ChunkHandler, g)
		RegisterSessionRoutes(v1, params.SessionHandler, g)
		RegisterChatRoutes(v1, params.SessionHandler, g)
		RegisterMessageRoutes(v1, params.MessageHandler, g)
		RegisterModelRoutes(v1, params.ModelHandler, params.ModelCredentialsHandler, g)
		RegisterEvaluationRoutes(v1, params.EvaluationHandler, g)
		RegisterInitializationRoutes(v1, params.InitializationHandler, g)
		RegisterSystemRoutes(v1, params.SystemHandler, g)
		RegisterSystemAdminRoutes(v1, params.SystemHandler, params.AuditLogHandler, g)
		RegisterMCPServiceRoutes(v1, params.MCPServiceHandler, params.MCPCredentialsHandler, params.MCPOAuthHandler, g)
		RegisterWebSearchRoutes(v1, params.WebSearchHandler, g)
		RegisterWebSearchProviderRoutes(v1, params.WebSearchProviderHandler, params.WebSearchCredentialsHandler, g)
		RegisterVectorStoreRoutes(v1, params.VectorStoreHandler, g)
		RegisterCustomAgentRoutes(v1, params.CustomAgentHandler, g)
		RegisterUserFavoriteRoutes(v1, params.UserFavoriteHandler, g)
		RegisterSkillRoutes(v1, params.SkillHandler, g)
		RegisterOrganizationRoutes(v1, params.OrganizationHandler, g)
		RegisterIMChannelRoutes(v1, params.IMHandler, g)
		RegisterEmbedChannelRoutes(v1, params.EmbedChannelHandler, g)
		RegisterDataSourceRoutes(v1, params.DataSourceHandler, params.DataSourceCredentialsHandler, g)
		RegisterWeKnoraCloudRoutes(v1, params.WeKnoraCloudHandler, g)
		RegisterWikiPageRoutes(v1, params.WikiPageHandler, g)
		RegisterChunkerDebugRoutes(v1, g)
		RegisterExportRoutes(v1, params.ExportHandler)

		// Org-tree management routes (accessible by super admin and org admin, permission enforced in handler)
		RegisterOrgTreeRoutes(v1, params.OrgTreeHandler)

		// Super admin routes (require super admin privileges)
		superAdmin := v1.Group("", middleware.RequireSuperAdmin())
		{
			RegisterOrgTreeSuperAdminRoutes(superAdmin, params.OrgTreeHandler)
			RegisterModelWriteRoutes(superAdmin, params.ModelHandler)
		}

		// User org-tree membership route (accessible by all authenticated users)
		v1.GET("/my-organizations", params.OrgTreeHandler.GetMyOrganizations)
	}

	return r
}

// RegisterChunkerDebugRoutes wires the read-only chunker preview endpoint
// used by the KB editor's debug panel. Stateless — uses no service deps.
func RegisterChunkerDebugRoutes(r *gin.RouterGroup, g *rbacGuards) {
	r.POST("/chunker/preview", g.Viewer(), handler.PreviewChunking)
}

// RegisterChunkRoutes 注册分块相关的路由
func RegisterChunkRoutes(r *gin.RouterGroup, handler *handler.ChunkHandler, g *rbacGuards) {
	// 分块路由组
	chunks := r.Group("/chunks")
	{
		chunks.GET("/:knowledge_id", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("knowledge_id"), handler.ListKnowledgeChunks)
		chunks.GET("/by-id/:id", g.Viewer(), g.KBAccessReadFromChunkIDParam("id"), handler.GetChunkByIDOnly)
		chunks.DELETE("/:knowledge_id/:id", g.OwnedChunkKBOrAdmin(), g.KBAccessWriteFromKnowledgeIDParam("knowledge_id"), handler.DeleteChunk)
		chunks.DELETE("/:knowledge_id", g.OwnedChunkKBOrAdmin(), g.KBAccessWriteFromKnowledgeIDParam("knowledge_id"), handler.DeleteChunksByKnowledgeID)
		chunks.PUT("/:knowledge_id/:id", g.OwnedChunkKBOrAdmin(), g.KBAccessWriteFromKnowledgeIDParam("knowledge_id"), handler.UpdateChunk)
		chunks.DELETE("/by-id/:id/questions", g.OwnedChunkKBOrAdminFromChunkID(), g.KBAccessWriteFromChunkIDParam("id"), handler.DeleteGeneratedQuestion)
	}
}

// RegisterKnowledgeRoutes 注册知识相关的路由
func RegisterKnowledgeRoutes(r *gin.RouterGroup, handler *handler.KnowledgeHandler, g *rbacGuards) {
	// 知识库下的知识路由组
	kb := r.Group("/knowledge-bases/:id/knowledge")
	{
		kb.POST("/file", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.CreateKnowledgeFromFile)
		kb.POST("/url", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.CreateKnowledgeFromURL)
		kb.POST("/manual", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.CreateManualKnowledge)
		kb.GET("", g.Viewer(), g.KBAccessRead("id"), handler.ListKnowledge)
		kb.DELETE("", g.Admin(), g.KBAccessWrite("id"), handler.ClearKnowledgeBaseContents)
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
		k.POST("/batch-reparse", g.Contributor(), handler.BatchReparseKnowledge)
		k.GET("/search", g.Viewer(), handler.SearchKnowledge)
		k.POST("/batch-delete", g.Contributor(), handler.BatchDeleteKnowledge)
		k.POST("/move", g.Contributor(), handler.MoveKnowledge)
		k.GET("/move/progress/:task_id", g.Viewer(), handler.GetKnowledgeMoveProgress)
	}
}

// RegisterFAQRoutes 注册 FAQ 相关路由
func RegisterFAQRoutes(r *gin.RouterGroup, handler *handler.FAQHandler, g *rbacGuards) {
	if handler == nil {
		return
	}
	faq := r.Group("/knowledge-bases/:id/faq")
	{
		faq.GET("/entries", g.Viewer(), g.KBAccessRead("id"), handler.ListEntries)
		faq.GET("/entries/export", g.Viewer(), g.KBAccessRead("id"), handler.ExportEntries)
		faq.GET("/entries/:entry_id", g.Viewer(), g.KBAccessRead("id"), handler.GetEntry)
		faq.POST("/entries", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.UpsertEntries)
		faq.POST("/entry", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.CreateEntry)
		faq.PUT("/entries/:entry_id", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.UpdateEntry)
		faq.POST("/entries/:entry_id/similar-questions", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.AddSimilarQuestions)
		faq.PUT("/entries/fields", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.UpdateEntryFieldsBatch)
		faq.PUT("/entries/tags", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.UpdateEntryTagBatch)
		faq.DELETE("/entries", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.DeleteEntries)
		faq.POST("/search", g.Viewer(), g.KBAccessRead("id"), handler.SearchFAQ)
		faq.PUT("/import/last-result/display", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.UpdateLastImportResultDisplayStatus)
	}
	// FAQ import progress route (outside of knowledge-base scope)
	faqImport := r.Group("/faq/import")
	{
		faqImport.GET("/progress/:task_id", g.Viewer(), handler.GetImportProgress)
	}
}

// RegisterKnowledgeBaseRoutes 注册知识库相关的路由
func RegisterKnowledgeBaseRoutes(r *gin.RouterGroup, handler *handler.KnowledgeBaseHandler, g *rbacGuards) {
	// 知识库路由组
	kb := r.Group("/knowledge-bases")
	{
		kb.POST("", g.Contributor(), handler.CreateKnowledgeBase)
		kb.GET("", g.Viewer(), handler.ListKnowledgeBases)
		kb.GET("/:id", g.Viewer(), g.KBAccessRead("id"), handler.GetKnowledgeBase)
		kb.PUT("/:id", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.UpdateKnowledgeBase)
		kb.DELETE("/:id", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), handler.DeleteKnowledgeBase)
		kb.PUT("/:id/pin", g.Viewer(), g.KBAccessRead("id"), handler.TogglePinKnowledgeBase)
		kb.POST("/:id/hybrid-search", g.Viewer(), g.KBAccessRead("id"), handler.HybridSearch)
		kb.GET("/:id/hybrid-search", g.Viewer(), g.KBAccessRead("id"), handler.HybridSearch)
		kb.POST("/copy", g.Contributor(), handler.CopyKnowledgeBase)
		kb.GET("/copy/progress/:task_id", g.Viewer(), handler.GetKBCloneProgress)
		kb.GET("/:id/move-targets", g.Viewer(), g.KBAccessRead("id"), handler.ListMoveTargets)
	}
}

// RegisterKnowledgeTagRoutes 注册知识库标签相关路由
func RegisterKnowledgeTagRoutes(r *gin.RouterGroup, tagHandler *handler.TagHandler, g *rbacGuards) {
	if tagHandler == nil {
		return
	}
	kbTags := r.Group("/knowledge-bases/:id/tags")
	{
		kbTags.GET("", g.Viewer(), g.KBAccessRead("id"), tagHandler.ListTags)
		kbTags.POST("", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), tagHandler.CreateTag)
		kbTags.PUT("/:tag_id", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), tagHandler.UpdateTag)
		kbTags.DELETE("/:tag_id", g.OwnedKBOrAdmin(), g.KBAccessWrite("id"), tagHandler.DeleteTag)
	}
}

// RegisterMessageRoutes 注册消息相关的路由
func RegisterMessageRoutes(r *gin.RouterGroup, handler *handler.MessageHandler, g *rbacGuards) {
	// 消息路由组
	messages := r.Group("/messages")
	{
		messages.POST("/search", g.Viewer(), handler.SearchMessages)
		messages.GET("/chat-history-stats", g.Viewer(), handler.GetChatHistoryKBStats)
		messages.GET("/:session_id/load", g.Viewer(), handler.LoadMessages)
		messages.DELETE("/:session_id/:id", g.Viewer(), handler.DeleteMessage)
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
func RegisterSessionRoutes(r *gin.RouterGroup, handler *session.Handler, g *rbacGuards) {
	sessions := r.Group("/sessions", g.Viewer())
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
func RegisterChatRoutes(r *gin.RouterGroup, handler *session.Handler, g *rbacGuards) {
	knowledgeChat := r.Group("/knowledge-chat", g.Viewer())
	{
		knowledgeChat.POST("/:session_id", handler.KnowledgeQA)
	}

	// Agent-based chat
	agentChat := r.Group("/agent-chat", g.Viewer())
	{
		agentChat.POST("/:session_id", handler.AgentQA)
	}

	chatDocumentArtifacts := r.Group("/chat-document-artifacts", g.Viewer())
	{
		chatDocumentArtifacts.GET("", handler.ListChatDocumentArtifacts)
		chatDocumentArtifacts.GET("/latest", handler.GetLatestChatDocumentArtifact)
		chatDocumentArtifacts.GET("/:artifact_id", handler.GetChatDocumentArtifact)
		chatDocumentArtifacts.GET("/:artifact_id/revisions", handler.ListChatDocumentArtifactRevisions)
	}

	// 新增知识检索接口，不需要session_id
	knowledgeSearch := r.Group("/knowledge-search", g.Viewer())
	{
		knowledgeSearch.POST("", handler.SearchKnowledge)
	}
}

// RegisterTenantRoutes registers tenant routes, including tenant-member,
// invitation and audit-log subresources.
func RegisterTenantRoutes(
	r *gin.RouterGroup,
	handler *handler.TenantHandler,
	memberHandler *handler.TenantMemberHandler,
	invitationHandler *handler.TenantInvitationHandler,
	auditLogHandler *handler.AuditLogHandler,
	g *rbacGuards,
) {
	r.GET("/tenants/all", g.CrossTenant(), handler.ListAllTenants)
	r.GET("/tenants/search", g.CrossTenant(), handler.SearchTenants)
	tenantRoutes := r.Group("/tenants")
	{
		tenantRoutes.POST("", handler.CreateTenant)
		tenantRoutes.GET("", handler.ListTenants)
		tenantRoutes.GET("/kv/:key", g.Viewer(), handler.GetTenantKV)
		tenantRoutes.PUT("/kv/:key", g.Admin(), handler.UpdateTenantKV)

		tenantByID := tenantRoutes.Group("/:id", g.PathTenantMatch())
		{
			tenantByID.GET("", g.Viewer(), handler.GetTenant)
			tenantByID.PUT("", g.Owner(), handler.UpdateTenant)
			tenantByID.DELETE("", g.Owner(), handler.DeleteTenant)
			tenantByID.POST("/api-key", g.Owner(), handler.ResetAPIKey)
			tenantByID.GET("/api-principal-config", g.Owner(), handler.GetAPIPrincipalConfig)
			tenantByID.PUT("/api-principal-config", g.Owner(), handler.UpdateAPIPrincipalConfig)

			if memberHandler != nil {
				tenantByID.GET("/members", g.Viewer(), memberHandler.ListMembers)
				tenantByID.POST("/members", g.Owner(), memberHandler.AddMember)
				tenantByID.PUT("/members/:user_id", g.Owner(), memberHandler.UpdateMemberRole)
				tenantByID.DELETE("/members/:user_id", g.Owner(), memberHandler.RemoveMember)
				tenantByID.POST("/leave", g.Viewer(), memberHandler.LeaveTenant)
			}

			if invitationHandler != nil {
				tenantByID.GET("/invitations", g.Viewer(), invitationHandler.ListTenantInvitations)
				tenantByID.POST("/invitations", g.Owner(), invitationHandler.CreateInvitation)
				tenantByID.DELETE("/invitations/:inv_id", g.Owner(), invitationHandler.RevokeInvitation)
				tenantByID.POST("/invite-links", g.Owner(), invitationHandler.CreateInviteLink)
			}

			if auditLogHandler != nil {
				tenantByID.GET("/audit-log", g.Admin(), auditLogHandler.ListTenantAuditLog)
			}
		}
	}
}

// RegisterMyInvitationRoutes wires the current user's invitation inbox.
func RegisterMyInvitationRoutes(r *gin.RouterGroup, invitationHandler *handler.TenantInvitationHandler) {
	if invitationHandler == nil {
		return
	}
	meRoutes := r.Group("/me")
	{
		meRoutes.GET("/invitations", invitationHandler.ListMyInvitations)
		meRoutes.GET("/invitations/pending-count", invitationHandler.CountMyPendingInvitations)
		meRoutes.POST("/invitations/:inv_id/accept", invitationHandler.AcceptMyInvitation)
		meRoutes.POST("/invitations/:inv_id/decline", invitationHandler.DeclineMyInvitation)
	}
}

// RegisterTenantWriteRoutes registers tenant KV write routes (super admin only)
// NOTE: This function is now unused since KV write routes have been moved to RegisterTenantRoutes
// to allow normal users to update their own tenant's configuration
func RegisterTenantWriteRoutes(r *gin.RouterGroup, handler *handler.TenantHandler) {
	// Moved to RegisterTenantRoutes
}

// RegisterModelRoutes registers model routes.
func RegisterModelRoutes(r *gin.RouterGroup, handler *handler.ModelHandler, credHandler *handler.ModelCredentialsHandler, g *rbacGuards) {
	models := r.Group("/models")
	{
		models.GET("/providers", g.Viewer(), handler.ListModelProviders)
		models.POST("", g.Admin(), handler.CreateModel)
		models.GET("", g.Viewer(), handler.ListModels)
		models.POST("/:id/debug", g.Admin(), handler.DebugModel)
		models.GET("/:id", g.Viewer(), handler.GetModel)
		models.PUT("/:id", g.Admin(), handler.UpdateModel)
		models.DELETE("/:id", g.Admin(), handler.DeleteModel)
		models.PUT("/:id/credentials", g.Admin(), credHandler.Put)
		models.DELETE("/:id/credentials/:field", g.Admin(), credHandler.DeleteField)
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

func RegisterEvaluationRoutes(r *gin.RouterGroup, handler *handler.EvaluationHandler, g *rbacGuards) {
	evaluationRoutes := r.Group("/evaluation")
	{
		evaluationRoutes.POST("/", g.Admin(), handler.Evaluation)
		evaluationRoutes.GET("/", g.Viewer(), handler.GetEvaluationResult)
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

func RegisterInitializationRoutes(r *gin.RouterGroup, handler *handler.InitializationHandler, g *rbacGuards) {
	r.GET("/initialization/config/:kbId", g.Viewer(), handler.GetCurrentConfigByKB)
	r.POST("/initialization/initialize/:kbId", g.OwnedKBOrAdminFromKbIDParam(), handler.InitializeByKB)
	r.PUT("/initialization/config/:kbId", g.OwnedKBOrAdminFromKbIDParam(), handler.UpdateKBConfig)

	r.GET("/initialization/ollama/status", g.Viewer(), handler.CheckOllamaStatus)
	r.GET("/initialization/ollama/models", g.Viewer(), handler.ListOllamaModels)
	r.POST("/initialization/ollama/models/check", g.Admin(), handler.CheckOllamaModels)
	r.POST("/initialization/ollama/models/download", g.Admin(), handler.DownloadOllamaModel)
	r.GET("/initialization/ollama/download/progress/:taskId", g.Viewer(), handler.GetDownloadProgress)
	r.GET("/initialization/ollama/download/tasks", g.Viewer(), handler.ListDownloadTasks)

	r.POST("/initialization/remote/check", g.Admin(), handler.CheckRemoteModel)
	r.POST("/initialization/embedding/test", g.Admin(), handler.TestEmbeddingModel)
	r.POST("/initialization/rerank/check", g.Admin(), handler.CheckRerankModel)
	r.POST("/initialization/asr/check", g.Admin(), handler.CheckASRModel)
	r.POST("/initialization/multimodal/test", g.Admin(), handler.TestMultimodalFunction)

	r.POST("/initialization/extract/text-relation", g.Admin(), handler.ExtractTextRelations)
	r.POST("/initialization/extract/fabri-tag", g.Admin(), handler.FabriTag)
	r.POST("/initialization/extract/fabri-text", g.Admin(), handler.FabriText)
}

// RegisterSystemRoutes registers system information routes
func RegisterSystemRoutes(r *gin.RouterGroup, handler *handler.SystemHandler, g *rbacGuards) {
	systemRoutes := r.Group("/system")
	{
		systemRoutes.GET("/info", g.Viewer(), handler.GetSystemInfo)
		systemRoutes.GET("/parser-engines", g.Viewer(), handler.ListParserEngines)
		systemRoutes.POST("/parser-engines/check", g.Admin(), handler.CheckParserEngines)
		systemRoutes.POST("/docreader/reconnect", g.Admin(), handler.ReconnectDocReader)
		systemRoutes.GET("/storage-engine-status", g.Viewer(), handler.GetStorageEngineStatus)
		systemRoutes.POST("/storage-engine-check", g.Admin(), handler.CheckStorageEngine)
	}
}

// RegisterSystemAdminRoutes registers system-admin-only routes for global
// settings, system-admin management, and platform audit logs.
func RegisterSystemAdminRoutes(
	r *gin.RouterGroup,
	systemHandler *handler.SystemHandler,
	auditHandler *handler.AuditLogHandler,
	g *rbacGuards,
) {
	admin := r.Group("/system/admin", g.SystemAdmin())
	{
		admin.GET("/list", systemHandler.ListSystemAdmins)
		admin.POST("/promote", systemHandler.PromoteUserToSystemAdmin)
		admin.POST("/revoke", systemHandler.RevokeSystemAdmin)
		admin.GET("/settings", systemHandler.ListSystemSettings)
		admin.GET("/settings/:key", systemHandler.GetSystemSetting)
		admin.PUT("/settings/:key", systemHandler.UpdateSystemSetting)
		admin.DELETE("/settings/:key", systemHandler.ResetSystemSetting)
		admin.POST("/tenants/apply-default-storage-quota", systemHandler.ApplyDefaultStorageQuotaToAllTenants)
		if auditHandler != nil {
			admin.GET("/audit-log", auditHandler.ListSystemAuditLog)
		}
	}
}

func RegisterMCPServiceRoutes(
	r *gin.RouterGroup,
	handler *handler.MCPServiceHandler,
	credHandler *handler.MCPCredentialsHandler,
	oauthHandler *handler.MCPOAuthHandler,
	g *rbacGuards,
) {
	// MCP OAuth provider redirect. Registered OUTSIDE the /mcp-services group
	// to avoid a static-vs-":id" route conflict, and left unauthenticated
	// (allow-listed in middleware/auth.go) because the third-party browser
	// redirect carries no WeKnora bearer — the single-use state authenticates.
	r.GET("/mcp-oauth/callback", oauthHandler.Callback)

	mcpServices := r.Group("/mcp-services")
	{
		mcpServices.POST("", g.Admin(), handler.CreateMCPService)
		mcpServices.GET("", g.Viewer(), handler.ListMCPServices)
		mcpServices.GET("/:id", g.Viewer(), handler.GetMCPService)
		mcpServices.PUT("/:id", g.Admin(), handler.UpdateMCPService)
		mcpServices.DELETE("/:id", g.Admin(), handler.DeleteMCPService)
		mcpServices.POST("/:id/test", g.Admin(), handler.TestMCPService)
		mcpServices.GET("/:id/tools", g.Viewer(), handler.GetMCPServiceTools)
		mcpServices.GET("/:id/resources", g.Viewer(), handler.GetMCPServiceResources)
		mcpServices.PUT("/:id/credentials", g.Admin(), credHandler.Put)
		mcpServices.DELETE("/:id/credentials/:field", g.Admin(), credHandler.DeleteField)
		mcpServices.GET("/:id/tool-approvals", g.Viewer(), handler.ListMCPToolApprovals)
		mcpServices.PUT("/:id/tool-approvals/:tool_name", g.Admin(), handler.SetMCPToolApproval)
		mcpServices.POST("/:id/oauth/authorize-url", g.Viewer(), oauthHandler.AuthorizeURL)
		mcpServices.GET("/:id/oauth/status", g.Viewer(), oauthHandler.Status)
		mcpServices.DELETE("/:id/oauth/token", g.Viewer(), oauthHandler.Revoke)
	}

	agentTool := r.Group("/agent")
	{
		agentTool.POST("/tool-approvals/:pending_id", g.Viewer(), handler.ResolveToolApproval)
		agentTool.POST("/mcp-oauth-resolutions/:pending_id", g.Viewer(), oauthHandler.ResolveMCPOAuth)
		agentTool.POST("/mcp-oauth-resolutions/:pending_id/cancel", g.Viewer(), oauthHandler.CancelMCPOAuth)
	}
}

// RegisterWebSearchRoutes registers web search routes
func RegisterWebSearchRoutes(r *gin.RouterGroup, webSearchHandler *handler.WebSearchHandler, g *rbacGuards) {
	webSearch := r.Group("/web-search")
	{
		webSearch.GET("/providers", g.Viewer(), webSearchHandler.GetProviders)
	}
}

// RegisterWebSearchProviderRoutes registers CRUD routes for web search provider configurations
func RegisterWebSearchProviderRoutes(r *gin.RouterGroup, h *handler.WebSearchProviderHandler, credHandler *handler.WebSearchProviderCredentialsHandler, g *rbacGuards) {
	providers := r.Group("/web-search-providers")
	{
		providers.GET("/types", g.Viewer(), h.ListProviderTypes)
		providers.POST("/test", g.Admin(), h.TestProviderRaw)
		providers.POST("", g.Admin(), h.CreateProvider)
		providers.GET("", g.Viewer(), h.ListProviders)
		providers.GET("/:id", g.Viewer(), h.GetProvider)
		providers.PUT("/:id", g.Admin(), h.UpdateProvider)
		providers.DELETE("/:id", g.Admin(), h.DeleteProvider)
		providers.PUT("/:id/credentials", g.Admin(), credHandler.Put)
		providers.DELETE("/:id/credentials/:field", g.Admin(), credHandler.DeleteField)
		providers.POST("/:id/test", g.Admin(), h.TestProviderByID)
	}
}

// RegisterVectorStoreRoutes registers CRUD routes for vector store configurations
func RegisterVectorStoreRoutes(r *gin.RouterGroup, h *handler.VectorStoreHandler, g *rbacGuards) {
	stores := r.Group("/vector-stores")
	{
		stores.GET("/types", g.Viewer(), h.ListStoreTypes)
		stores.POST("/test", g.Admin(), h.TestStoreRaw)
		stores.POST("", g.Admin(), h.CreateStore)
		stores.GET("", g.Viewer(), h.ListStores)
		stores.GET("/:id", g.Viewer(), h.GetStore)
		stores.PUT("/:id", g.Admin(), h.UpdateStore)
		stores.DELETE("/:id", g.Admin(), h.DeleteStore)
		stores.POST("/:id/test", g.Admin(), h.TestStoreByID)
	}
}

// RegisterCustomAgentRoutes registers custom agent routes
func RegisterCustomAgentRoutes(r *gin.RouterGroup, agentHandler *handler.CustomAgentHandler, g *rbacGuards) {
	agents := r.Group("/agents")
	{
		agents.GET("/placeholders", g.Viewer(), agentHandler.GetPlaceholders)
		agents.GET("/type-presets", g.Viewer(), agentHandler.GetAgentTypePresets)
		agents.POST("", g.Contributor(), agentHandler.CreateAgent)
		agents.GET("", g.Viewer(), agentHandler.ListAgents)
		agents.GET("/:id/page-share", g.OwnedAgentOrAdmin(), agentHandler.GetAgentPageShare)
		agents.POST("/:id/page-share", g.OwnedAgentOrAdmin(), agentHandler.CreateOrEnableAgentPageShare)
		agents.DELETE("/:id/page-share", g.OwnedAgentOrAdmin(), agentHandler.DeleteAgentPageShare)
		agents.GET("/:id", g.Viewer(), agentHandler.GetAgent)
		agents.PUT("/:id", g.OwnedAgentOrAdmin(), agentHandler.UpdateAgent)
		agents.DELETE("/:id", g.OwnedAgentOrAdmin(), agentHandler.DeleteAgent)
		agents.POST("/:id/copy", g.Contributor(), agentHandler.CopyAgent)
	}
	// Registered outside the group to avoid Gin route conflict with /agents/:id/shares in organization routes
	r.GET("/agents/:id/suggested-questions", g.Viewer(), agentHandler.GetSuggestedQuestions)
}

// RegisterUserFavoriteRoutes wires the per-user starred-resource endpoints.
func RegisterUserFavoriteRoutes(r *gin.RouterGroup, h *handler.UserResourceFavoriteHandler, g *rbacGuards) {
	favs := r.Group("/user/favorites")
	{
		favs.GET("", g.Viewer(), h.ListFavorites)
		favs.POST("", g.Viewer(), h.AddFavorite)
		favs.DELETE("/:type/:id", g.Viewer(), h.RemoveFavorite)
	}
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
func RegisterSkillRoutes(r *gin.RouterGroup, skillHandler *handler.SkillHandler, g *rbacGuards) {
	skills := r.Group("/skills")
	{
		skills.GET("", g.Viewer(), skillHandler.ListSkills)
	}
}

// RegisterOrganizationRoutes registers organization and sharing routes
func RegisterOrganizationRoutes(r *gin.RouterGroup, orgHandler *handler.OrganizationHandler, g *rbacGuards) {
	// Organization routes
	orgs := r.Group("/organizations")
	{
		orgs.POST("", g.Admin(), orgHandler.CreateOrganization)
		orgs.GET("", g.Viewer(), orgHandler.ListMyOrganizations)
		orgs.GET("/preview/:code", g.Viewer(), orgHandler.PreviewByInviteCode)
		orgs.POST("/join", g.Admin(), orgHandler.JoinByInviteCode)
		orgs.POST("/join-request", g.Admin(), orgHandler.SubmitJoinRequest)
		orgs.GET("/search", g.Viewer(), orgHandler.SearchOrganizations)
		orgs.POST("/join-by-id", g.Admin(), orgHandler.JoinByOrganizationID)
		orgs.GET("/:id", g.Viewer(), orgHandler.GetOrganization)
		orgs.PUT("/:id", g.Admin(), orgHandler.UpdateOrganization)
		orgs.DELETE("/:id", g.Admin(), orgHandler.DeleteOrganization)
		orgs.POST("/:id/leave", g.Admin(), orgHandler.LeaveOrganization)
		orgs.POST("/:id/request-upgrade", g.Admin(), orgHandler.RequestRoleUpgrade)
		orgs.POST("/:id/invite-code", g.Admin(), orgHandler.GenerateInviteCode)
		orgs.GET("/:id/search-users", g.Admin(), orgHandler.SearchUsersForInvite)
		orgs.POST("/:id/invite", g.Admin(), orgHandler.InviteMember)
		orgs.GET("/:id/members", g.Viewer(), orgHandler.ListMembers)
		orgs.PUT("/:id/members/:user_id", g.Admin(), orgHandler.UpdateMemberRole)
		orgs.DELETE("/:id/members/:user_id", g.Admin(), orgHandler.RemoveMember)
		orgs.GET("/:id/join-requests", g.Admin(), orgHandler.ListJoinRequests)
		orgs.PUT("/:id/join-requests/:request_id/review", g.Admin(), orgHandler.ReviewJoinRequest)
		orgs.GET("/:id/shares", g.Viewer(), orgHandler.ListOrgShares)
		orgs.GET("/:id/agent-shares", g.Viewer(), orgHandler.ListOrgAgentShares)
		orgs.GET("/:id/shared-knowledge-bases", g.Viewer(), orgHandler.ListOrganizationSharedKnowledgeBases)
		orgs.GET("/:id/shared-agents", g.Viewer(), orgHandler.ListOrganizationSharedAgents)
	}

	// Knowledge base sharing routes (add to existing kb routes)
	kbShares := r.Group("/knowledge-bases/:id/shares")
	{
		kbShares.POST("", g.OwnedKBOrAdmin(), orgHandler.ShareKnowledgeBase)
		kbShares.GET("", g.Viewer(), orgHandler.ListKBShares)
		kbShares.PUT("/:share_id", g.OwnedKBOrAdmin(), orgHandler.UpdateSharePermission)
		kbShares.DELETE("/:share_id", g.OwnedKBOrAdmin(), orgHandler.RemoveShare)
	}

	// Agent sharing routes
	agentShares := r.Group("/agents/:id/shares")
	{
		agentShares.POST("", g.OwnedAgentOrAdmin(), orgHandler.ShareAgent)
		agentShares.GET("", g.OwnedAgentOrAdmin(), orgHandler.ListAgentShares)
		agentShares.DELETE("/:share_id", g.OwnedAgentOrAdmin(), orgHandler.RemoveAgentShare)
	}

	r.GET("/shared-knowledge-bases", g.Viewer(), orgHandler.ListSharedKnowledgeBases)
	r.GET("/shared-agents", g.Viewer(), orgHandler.ListSharedAgents)
	r.POST("/shared-agents/disabled", g.Admin(), orgHandler.SetSharedAgentDisabledByMe)
}

// RegisterEmbedPublicRoutes registers anonymous embed endpoints secured by publish tokens.
func RegisterEmbedPublicRoutes(
	r *gin.Engine,
	embedHandler *handler.EmbedChannelHandler,
	embedService interfaces.EmbedChannelService,
	tenantService interfaces.TenantService,
	redisClient *redis.Client,
	fileService interfaces.FileService,
) {
	if embedHandler == nil || embedService == nil {
		return
	}
	embed := r.Group("/api/v1/embed/:channel_id", middleware.EmbedAuth(embedService, tenantService, redisClient))
	{
		embed.POST("/exchange", embedHandler.ExchangeEmbedSession)
		embed.GET("/config", embedHandler.GetEmbedConfig)
		embed.GET("/suggested-questions", embedHandler.GetEmbedSuggestedQuestions)
		embed.GET("/chunks/:chunk_id", embedHandler.GetEmbedChunk)
		embed.POST("/sessions", embedHandler.CreateEmbedSession)
		embed.POST("/knowledge-chat/:session_id", embedHandler.EmbedKnowledgeChat)
		embed.POST("/agent-chat/:session_id", embedHandler.EmbedAgentChat)
		embed.GET("/messages/:session_id/load", embedHandler.EmbedLoadMessages)
		embed.POST("/sessions/:session_id/stop", embedHandler.EmbedStopSession)
		embed.POST("/sessions/:session_id/events", embedHandler.EmbedRelayWebhookEvent)
		embed.POST("/sessions/:session_id/mcp-oauth-resolutions/:pending_id", embedHandler.EmbedResolveMCPOAuth)
		embed.POST("/sessions/:session_id/mcp-oauth-resolutions/:pending_id/cancel", embedHandler.EmbedCancelMCPOAuth)
		embed.POST("/sessions/:session_id/mcp-services/:id/oauth/authorize-url", embedHandler.EmbedMCPOAuthAuthorizeURL)
		embed.GET("/sessions/:session_id/mcp-services/:id/oauth/status", embedHandler.EmbedMCPOAuthStatus)
		embed.POST("/sessions/:session_id/tool-approvals/:pending_id", embedHandler.EmbedResolveToolApproval)
		// Serve images embedded in bot replies (e.g. chart exports). EmbedAuth
		// injects the channel's tenant, and the handler enforces that the
		// requested path belongs to that tenant.
		embed.GET("/files", newFileServeHandler(fileService))
	}
}

// RegisterEmbedChannelRoutes registers authenticated embed channel management routes.
func RegisterEmbedChannelRoutes(r *gin.RouterGroup, embedHandler *handler.EmbedChannelHandler, g *rbacGuards) {
	if embedHandler == nil {
		return
	}
	agentEmbed := r.Group("/agents/:id/embed-channels")
	{
		agentEmbed.POST("", g.Admin(), embedHandler.CreateEmbedChannel)
		agentEmbed.GET("", g.Viewer(), embedHandler.ListEmbedChannels)
	}
	channels := r.Group("/embed-channels")
	{
		channels.GET("", g.Viewer(), embedHandler.ListAllEmbedChannels)
		channels.GET("/:channel_id", g.Viewer(), embedHandler.GetEmbedChannel)
		channels.PUT("/:channel_id", g.Admin(), embedHandler.UpdateEmbedChannel)
		channels.DELETE("/:channel_id", g.Admin(), embedHandler.DeleteEmbedChannel)
		channels.POST("/:channel_id/rotate-token", g.Admin(), embedHandler.RotateEmbedToken)
		channels.POST("/:channel_id/preview-session", g.Viewer(), embedHandler.IssuePreviewSession)
		channels.GET("/:channel_id/stats", g.Viewer(), embedHandler.GetEmbedChannelStats)
	}
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
func RegisterIMChannelRoutes(r *gin.RouterGroup, imHandler *handler.IMHandler, g *rbacGuards) {
	// Channel CRUD under agents
	agentChannels := r.Group("/agents/:id/im-channels")
	{
		agentChannels.POST("", g.Admin(), imHandler.CreateIMChannel)
		agentChannels.GET("", g.Viewer(), imHandler.ListIMChannels)
	}

	// Channel operations by channel ID
	channels := r.Group("/im-channels")
	{
		channels.GET("", g.Viewer(), imHandler.ListAllIMChannels)
		channels.PUT("/:id", g.Admin(), imHandler.UpdateIMChannel)
		channels.DELETE("/:id", g.Admin(), imHandler.DeleteIMChannel)
		channels.POST("/:id/toggle", g.Admin(), imHandler.ToggleIMChannel)
	}

	// WeChat QR code login (requires authentication)
	wechatGroup := r.Group("/wechat")
	{
		wechatGroup.POST("/qrcode", g.Admin(), imHandler.WeChatGetQRCode)
		wechatGroup.POST("/qrcode/status", g.Admin(), imHandler.WeChatPollQRCodeStatus)
	}
}

// trustedProxies returns the proxy CIDRs/IPs whose X-Forwarded-For headers
// gin should trust when resolving the client IP. Defaults to loopback and
// private ranges (covers the bundled nginx in a container network); override
// with WEKNORA_TRUSTED_PROXIES (comma-separated). An explicit empty value
// disables proxy trust entirely so ClientIP() returns the direct peer.
func trustedProxies() []string {
	raw, ok := os.LookupEnv("WEKNORA_TRUSTED_PROXIES")
	if !ok {
		return []string{
			"127.0.0.0/8",
			"::1/128",
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"fc00::/7",
		}
	}
	proxies := make([]string, 0)
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			proxies = append(proxies, p)
		}
	}
	return proxies
}

// embedChannelIDFromPath extracts the channel id from an /embed/:channelID path.
func embedChannelIDFromPath(path string) string {
	const prefix = "/embed/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}

// embedFrameAncestorsMiddleware sets a per-channel `frame-ancestors` CSP on the
// embed SPA page so it can only be framed by the channel's allowed origins.
// When the channel declares no origins (or "*"), no restriction is applied,
// matching the API allowlist semantics. Only GET/HEAD page loads are handled.
func embedFrameAncestorsMiddleware(svc interfaces.EmbedChannelService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		channelID := embedChannelIDFromPath(c.Request.URL.Path)
		if channelID == "" {
			c.Next()
			return
		}
		ch, err := svc.LookupEnabledChannel(c.Request.Context(), channelID)
		if err != nil || ch == nil {
			c.Next()
			return
		}
		origins := ch.AllowedOriginsList()
		sources := make([]string, 0, len(origins))
		wildcard := false
		for _, o := range origins {
			o = strings.TrimSpace(o)
			if o == "" {
				continue
			}
			if o == "*" {
				wildcard = true
				break
			}
			sources = append(sources, o)
		}
		// No explicit origins or a wildcard => do not constrain framing here.
		if wildcard || len(sources) == 0 {
			c.Next()
			return
		}
		c.Header("Content-Security-Policy", "frame-ancestors "+strings.Join(sources, " "))
		c.Next()
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

// newFileServeHandler builds the file-proxy handler. It reads the tenant from
// the request context (set by whichever auth middleware precedes it), so the
// same handler backs both the authenticated /files route and the embed route
// (where EmbedAuth injects the channel's tenant). Tenant ownership of the
// requested path is enforced via ValidateStoragePathTenant either way.
func newFileServeHandler(globalFileService interfaces.FileService) gin.HandlerFunc {
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

	return func(c *gin.Context) {
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
	}
}

func serveFiles(r getRouteRegistrar, globalFileService interfaces.FileService) {
	logger.Infof(context.Background(), "[Router] Serving files from /files")
	r.GET("/files", newFileServeHandler(globalFileService))
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
func RegisterDataSourceRoutes(r *gin.RouterGroup, handler *handler.DataSourceHandler, credHandler *handler.DataSourceCredentialsHandler, g *rbacGuards) {
	r.GET("/knowledge-bases/:id/database-schema", g.Viewer(), g.KBAccessRead("id"), handler.GetDatabaseSchema)
	r.GET("/database-query-audits", g.Viewer(), handler.ListDatabaseQueryAudits)

	// Data source routes
	ds := r.Group("/datasource")
	{
		ds.POST("/:id/validate", g.Admin(), handler.ValidateConnection)
		ds.POST("/:id/refresh-schema", g.Admin(), handler.RefreshSchema)
		ds.GET("/:id/resources", g.Admin(), handler.ListAvailableResources)
		ds.PUT("/:id/credentials", g.Admin(), credHandler.Put)
		ds.DELETE("/:id/credentials/:field", g.Admin(), credHandler.DeleteField)
		ds.POST("/:id/resource-ancestors", g.Admin(), handler.ResolveResourceAncestors)

		ds.GET("/types", g.Viewer(), handler.GetAvailableConnectors)
		ds.POST("/validate-credentials", g.Admin(), handler.ValidateCredentials)
		ds.POST("", g.Admin(), handler.CreateDataSource)
		ds.GET("", g.Viewer(), handler.ListDataSources)
		ds.GET("/:id", g.Viewer(), handler.GetDataSource)
		ds.PUT("/:id", g.Admin(), handler.UpdateDataSource)
		ds.DELETE("/:id", g.Admin(), handler.DeleteDataSource)
		ds.POST("/:id/sync", g.Admin(), handler.ManualSync)
		ds.POST("/:id/pause", g.Admin(), handler.PauseDataSource)
		ds.POST("/:id/resume", g.Admin(), handler.ResumeDataSource)

		// Sync logs
		ds.GET("/:id/logs", g.Viewer(), handler.GetSyncLogs)
		ds.GET("/logs/:log_id", g.Viewer(), handler.GetSyncLog)
	}
}

// RegisterWeKnoraCloudRoutes 注册 WeKnoraCloud 初始化路由
func RegisterWeKnoraCloudRoutes(r *gin.RouterGroup, handler *handler.WeKnoraCloudHandler, g *rbacGuards) {
	r.POST("/weknoracloud/credentials", g.Admin(), handler.SaveCredentials)
	r.GET("/models/weknoracloud/status", g.Viewer(), handler.Status)
}

// RegisterWikiPageRoutes registers wiki page related routes
func RegisterWikiPageRoutes(r *gin.RouterGroup, wikiHandler *handler.WikiPageHandler, g *rbacGuards) {
	wiki := r.Group("/knowledgebase/:kb_id/wiki")
	{
		// Page CRUD
		wiki.GET("/pages", g.Viewer(), wikiHandler.ListPages)
		wiki.POST("/pages", g.OwnedWikiKBOrAdmin(), wikiHandler.CreatePage)
		wiki.PUT("/move-page", g.OwnedWikiKBOrAdmin(), wikiHandler.MovePage)
		wiki.GET("/pages/*slug", g.Viewer(), wikiHandler.GetPage)
		wiki.PUT("/pages/*slug", g.OwnedWikiKBOrAdmin(), wikiHandler.UpdatePage)
		wiki.DELETE("/pages/*slug", g.OwnedWikiKBOrAdmin(), wikiHandler.DeletePage)

		// Folder tree (directory nodes)
		wiki.GET("/folders", g.Viewer(), wikiHandler.ListFolders)
		wiki.POST("/folders", g.OwnedWikiKBOrAdmin(), wikiHandler.CreateFolder)
		wiki.PUT("/folders/:folder_id", g.OwnedWikiKBOrAdmin(), wikiHandler.UpdateFolder)
		wiki.DELETE("/folders/:folder_id", g.OwnedWikiKBOrAdmin(), wikiHandler.DeleteFolder)

		// Special pages
		wiki.GET("/index", g.Viewer(), wikiHandler.GetIndex)
		wiki.GET("/log", g.Viewer(), wikiHandler.GetLog)
		wiki.GET("/graph", g.Viewer(), wikiHandler.GetGraph)
		wiki.GET("/stats", g.Viewer(), wikiHandler.GetStats)

		// Search and maintenance
		wiki.GET("/search", g.Viewer(), wikiHandler.SearchPages)
		wiki.POST("/rebuild-links", g.OwnedWikiKBOrAdmin(), wikiHandler.RebuildLinks)
		wiki.GET("/lint", g.Viewer(), wikiHandler.Lint)
		wiki.POST("/auto-fix", g.OwnedWikiKBOrAdmin(), wikiHandler.AutoFix)

		// Issues
		wiki.GET("/issues", g.Viewer(), wikiHandler.ListIssues)
		wiki.PUT("/issues/:issue_id/status", g.OwnedWikiKBOrAdmin(), wikiHandler.UpdateIssueStatus)
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
