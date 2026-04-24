tus_code=200]                      |
INFO [2026-04-24 15:36:28.998] [y7cqx3w2prcq client_ip=127.0.0.1 latency=3.445427ms method=GET path=/api/v1/knowledge-bases/bfeed109-3fcf-4cbc-8ce4-4291fedc40b1/tags?page=1&page_size=50 response_body={"data":{"total":0,"page":1,"page_size":50,"data":[]},"success":true} size=69 status_code=200]                      |
INFO [2026-04-24 15:36:30.035] [ document_process=8bf9ea36-c2be-4c14-9dec-dc06a15a530b] knowledge.go:7928[ProcessDocument] | Processing document task: knowledge_id=8bf9ea36-c2be-4c14-9dec-dc06a15a530b, file_path=minio://weknora/10000/8bf9ea36-c2be-4c14-9dec-dc06a15a530b/a2a8d175-9609-461d-ba19-c3620e53dc69.pdf, retry=0/3
INFO [2026-04-24 15:36:30.044] [ document_process=8bf9ea36-c2be-4c14-9dec-dc06a15a530b] knowledge.go:8312[convert] | [convert] kb=bfeed109-3fcf-4cbc-8ce4-4291fedc40b1 fileType=pdf isURL=false engine="mineru" rules=[{FileTypes:[pdf] Engine:mineru} {FileTypes:[docx doc] Engine:mineru} {FileTypes:[pptx ppt] Engine:mineru} {FileTypes:[xlsx xls] Engine:builtin} {FileTypes:[csv] Engine:simple} {FileTypes:[md markdown] Engine:builtin} {FileTypes:[txt] Engine:simple} {FileTypes:[json] Engine:simple} {FileTypes:[jpg jpeg png gif bmp tiff webp] Engine:mineru} {FileTypes:[mp3 wav m4a flac ogg] Engine:simple}]
INFO [2026-04-24 15:36:30.060] [ document_process=8bf9ea36-c2be-4c14-9dec-dc06a15a530b] knowledge.go:7693[resolveFileService] | [storage] resolveFileService selected: kb=bfeed109-3fcf-4cbc-8ce4-4291fedc40b1 provider=minio
INFO [2026-04-24 15:36:30.535] [] mineru_converter.go:60[Read] | [MinerU] Parsing file=20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf size=5365539 via http://127.0.0.1:8000
INFO [2026-04-24 15:36:30.653] [Y5NHQ4r9EIIB] knowledge.go:977[GetKnowledgeBatch] | Batch retrieving knowledge without kb_id, effective tenant ID: 10000, IDs count: 1

2026/04/24 15:36:30 /home/xmkp/workspace/WeKnora/internal/application/repository/knowledge.go:182
[0.703ms] [rows:1] SELECT * FROM "knowledges" WHERE (tenant_id = 10000 AND id IN ('8bf9ea36-c2be-4c14-9dec-dc06a15a530b')) AND "knowledges"."deleted_at" IS NULL
INFO [2026-04-24 15:36:30.654] [Y5NHQ4r9EIIB] knowledge.go:1011[GetKnowledgeBatch] | Batch knowledge retrieval successful, requested count: 1, returned count: 1
INFO [2026-04-24 15:36:30.654] [Y5NHQ4r9EIIB client_ip=127.0.0.1 latency=3.368047ms method=GET path=/api/v1/knowledge/batch?ids=8bf9ea36-c2be-4c14-9dec-dc06a15a530b& response_body={"data":[{"id":"8bf9ea36-c2be-4c14-9dec-dc06a15a530b","tenant_id":10000,"knowledge_base_id":"bfeed109-3fcf-4cbc-8ce4-4291fedc40b1","tag_id":"","type":"file","title":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","description":"","source":"","channel":"web","parse_status":"processing","summary_status":"none","enable_status":"disabled","embedding_model_id":"afa200dd-d0f3-460d-999b-c3e8a9e7d554","file_name":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","file_type":"pdf","file_size":5365539,"file_hash":"430dc0916b5caa5fcfa1841d6ee761f7","file_path":"minio://weknora/10000/8bf9ea36-c2be-4c14-9dec-dc06a15a530b/a2a8d175-9609-461d-ba19-c3620e53dc69.pdf","storage_size":0,"metadata":null,"last_faq_import_result":null,"created_at":"2026-04-24T15:36:28.354639+08:00","updated_at":"2026-04-24T15:36:30.038613+08:00","processed_at":null,"error_message":"","deleted_at":null,"knowledge_base_name":""}],"success":true} size=942 status_code=200]                      |
INFO [2026-04-24 15:36:32.558] [5KDcMGvWqQRD] knowledge.go:977[GetKnowledgeBatch] | Batch retrieving knowledge without kb_id, effective tenant ID: 10000, IDs count: 1

2026/04/24 15:36:32 /home/xmkp/workspace/WeKnora/internal/application/repository/knowledge.go:182
[0.809ms] [rows:1] SELECT * FROM "knowledges" WHERE (tenant_id = 10000 AND id IN ('8bf9ea36-c2be-4c14-9dec-dc06a15a530b')) AND "knowledges"."deleted_at" IS NULL
INFO [2026-04-24 15:36:32.559] [5KDcMGvWqQRD] knowledge.go:1011[GetKnowledgeBatch] | Batch knowledge retrieval successful, requested count: 1, returned count: 1
INFO [2026-04-24 15:36:32.559] [5KDcMGvWqQRD client_ip=127.0.0.1 latency=2.974825ms method=GET path=/api/v1/knowledge/batch?ids=8bf9ea36-c2be-4c14-9dec-dc06a15a530b& response_body={"data":[{"id":"8bf9ea36-c2be-4c14-9dec-dc06a15a530b","tenant_id":10000,"knowledge_base_id":"bfeed109-3fcf-4cbc-8ce4-4291fedc40b1","tag_id":"","type":"file","title":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","description":"","source":"","channel":"web","parse_status":"processing","summary_status":"none","enable_status":"disabled","embedding_model_id":"afa200dd-d0f3-460d-999b-c3e8a9e7d554","file_name":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","file_type":"pdf","file_size":5365539,"file_hash":"430dc0916b5caa5fcfa1841d6ee761f7","file_path":"minio://weknora/10000/8bf9ea36-c2be-4c14-9dec-dc06a15a530b/a2a8d175-9609-461d-ba19-c3620e53dc69.pdf","storage_size":0,"metadata":null,"last_faq_import_result":null,"created_at":"2026-04-24T15:36:28.354639+08:00","updated_at":"2026-04-24T15:36:30.038613+08:00","processed_at":null,"error_message":"","deleted_at":null,"knowledge_base_name":""}],"success":true} size=942 status_code=200]                      |
INFO [2026-04-24 15:36:34.485] [eBKbFDl0f5rl] knowledge.go:977[GetKnowledgeBatch] | Batch retrieving knowledge without kb_id, effective tenant ID: 10000, IDs count: 1

2026/04/24 15:36:34 /home/xmkp/workspace/WeKnora/internal/application/repository/knowledge.go:182
[0.559ms] [rows:1] SELECT * FROM "knowledges" WHERE (tenant_id = 10000 AND id IN ('8bf9ea36-c2be-4c14-9dec-dc06a15a530b')) AND "knowledges"."deleted_at" IS NULL
INFO [2026-04-24 15:36:34.486] [eBKbFDl0f5rl] knowledge.go:1011[GetKnowledgeBatch] | Batch knowledge retrieval successful, requested count: 1, returned count: 1
INFO [2026-04-24 15:36:34.486] [eBKbFDl0f5rl client_ip=127.0.0.1 latency=2.71592ms method=GET path=/api/v1/knowledge/batch?ids=8bf9ea36-c2be-4c14-9dec-dc06a15a530b& response_body={"data":[{"id":"8bf9ea36-c2be-4c14-9dec-dc06a15a530b","tenant_id":10000,"knowledge_base_id":"bfeed109-3fcf-4cbc-8ce4-4291fedc40b1","tag_id":"","type":"file","title":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","description":"","source":"","channel":"web","parse_status":"processing","summary_status":"none","enable_status":"disabled","embedding_model_id":"afa200dd-d0f3-460d-999b-c3e8a9e7d554","file_name":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","file_type":"pdf","file_size":5365539,"file_hash":"430dc0916b5caa5fcfa1841d6ee761f7","file_path":"minio://weknora/10000/8bf9ea36-c2be-4c14-9dec-dc06a15a530b/a2a8d175-9609-461d-ba19-c3620e53dc69.pdf","storage_size":0,"metadata":null,"last_faq_import_result":null,"created_at":"2026-04-24T15:36:28.354639+08:00","updated_at":"2026-04-24T15:36:30.038613+08:00","processed_at":null,"error_message":"","deleted_at":null,"knowledge_base_name":""}],"success":true} size=942 status_code=200]                      |
INFO [2026-04-24 15:36:36.382] [DEQj7IVKPLSn] knowledge.go:977[GetKnowledgeBatch] | Batch retrieving knowledge without kb_id, effective tenant ID: 10000, IDs count: 1

2026/04/24 15:36:36 /home/xmkp/workspace/WeKnora/internal/application/repository/knowledge.go:182
[0.571ms] [rows:1] SELECT * FROM "knowledges" WHERE (tenant_id = 10000 AND id IN ('8bf9ea36-c2be-4c14-9dec-dc06a15a530b')) AND "knowledges"."deleted_at" IS NULL
INFO [2026-04-24 15:36:36.382] [DEQj7IVKPLSn] knowledge.go:1011[GetKnowledgeBatch] | Batch knowledge retrieval successful, requested count: 1, returned count: 1
INFO [2026-04-24 15:36:36.382] [DEQj7IVKPLSn client_ip=127.0.0.1 latency=3.034993ms method=GET path=/api/v1/knowledge/batch?ids=8bf9ea36-c2be-4c14-9dec-dc06a15a530b& response_body={"data":[{"id":"8bf9ea36-c2be-4c14-9dec-dc06a15a530b","tenant_id":10000,"knowledge_base_id":"bfeed109-3fcf-4cbc-8ce4-4291fedc40b1","tag_id":"","type":"file","title":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","description":"","source":"","channel":"web","parse_status":"processing","summary_status":"none","enable_status":"disabled","embedding_model_id":"afa200dd-d0f3-460d-999b-c3e8a9e7d554","file_name":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","file_type":"pdf","file_size":5365539,"file_hash":"430dc0916b5caa5fcfa1841d6ee761f7","file_path":"minio://weknora/10000/8bf9ea36-c2be-4c14-9dec-dc06a15a530b/a2a8d175-9609-461d-ba19-c3620e53dc69.pdf","storage_size":0,"metadata":null,"last_faq_import_result":null,"created_at":"2026-04-24T15:36:28.354639+08:00","updated_at":"2026-04-24T15:36:30.038613+08:00","processed_at":null,"error_message":"","deleted_at":null,"knowledge_base_name":""}],"success":true} size=942 status_code=200]                      |
INFO [2026-04-24 15:36:41.161] [vMqWTMUlAjEC] knowledge.go:977[GetKnowledgeBatch] | Batch retrieving knowledge without kb_id, effective tenant ID: 10000, IDs count: 1

2026/04/24 15:36:41 /home/xmkp/workspace/WeKnora/internal/application/repository/knowledge.go:182
[17.934ms] [rows:1] SELECT * FROM "knowledges" WHERE (tenant_id = 10000 AND id IN ('8bf9ea36-c2be-4c14-9dec-dc06a15a530b')) AND "knowledges"."deleted_at" IS NULL
INFO [2026-04-24 15:36:41.183] [vMqWTMUlAjEC] knowledge.go:1011[GetKnowledgeBatch] | Batch knowledge retrieval successful, requested count: 1, returned count: 1
INFO [2026-04-24 15:36:41.185] [vMqWTMUlAjEC client_ip=127.0.0.1 latency=81.659111ms method=GET path=/api/v1/knowledge/batch?ids=8bf9ea36-c2be-4c14-9dec-dc06a15a530b& response_body={"data":[{"id":"8bf9ea36-c2be-4c14-9dec-dc06a15a530b","tenant_id":10000,"knowledge_base_id":"bfeed109-3fcf-4cbc-8ce4-4291fedc40b1","tag_id":"","type":"file","title":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","description":"","source":"","channel":"web","parse_status":"processing","summary_status":"none","enable_status":"disabled","embedding_model_id":"afa200dd-d0f3-460d-999b-c3e8a9e7d554","file_name":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","file_type":"pdf","file_size":5365539,"file_hash":"430dc0916b5caa5fcfa1841d6ee761f7","file_path":"minio://weknora/10000/8bf9ea36-c2be-4c14-9dec-dc06a15a530b/a2a8d175-9609-461d-ba19-c3620e53dc69.pdf","storage_size":0,"metadata":null,"last_faq_import_result":null,"created_at":"2026-04-24T15:36:28.354639+08:00","updated_at":"2026-04-24T15:36:30.038613+08:00","processed_at":null,"error_message":"","deleted_at":null,"knowledge_base_name":""}],"success":true} size=942 status_code=200]                      |
INFO [2026-04-24 15:36:43.590] [6x8DFxA1W5yg] knowledge.go:977[GetKnowledgeBatch] | Batch retrieving knowledge without kb_id, effective tenant ID: 10000, IDs count: 1

2026/04/24 15:36:43 /home/xmkp/workspace/WeKnora/internal/application/repository/knowledge.go:182
[0.421ms] [rows:1] SELECT * FROM "knowledges" WHERE (tenant_id = 10000 AND id IN ('8bf9ea36-c2be-4c14-9dec-dc06a15a530b')) AND "knowledges"."deleted_at" IS NULL
INFO [2026-04-24 15:36:43.591] [6x8DFxA1W5yg] knowledge.go:1011[GetKnowledgeBatch] | Batch knowledge retrieval successful, requested count: 1, returned count: 1
INFO [2026-04-24 15:36:43.591] [6x8DFxA1W5yg client_ip=127.0.0.1 latency=3.932095ms method=GET path=/api/v1/knowledge/batch?ids=8bf9ea36-c2be-4c14-9dec-dc06a15a530b& response_body={"data":[{"id":"8bf9ea36-c2be-4c14-9dec-dc06a15a530b","tenant_id":10000,"knowledge_base_id":"bfeed109-3fcf-4cbc-8ce4-4291fedc40b1","tag_id":"","type":"file","title":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","description":"","source":"","channel":"web","parse_status":"processing","summary_status":"none","enable_status":"disabled","embedding_model_id":"afa200dd-d0f3-460d-999b-c3e8a9e7d554","file_name":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","file_type":"pdf","file_size":5365539,"file_hash":"430dc0916b5caa5fcfa1841d6ee761f7","file_path":"minio://weknora/10000/8bf9ea36-c2be-4c14-9dec-dc06a15a530b/a2a8d175-9609-461d-ba19-c3620e53dc69.pdf","storage_size":0,"metadata":null,"last_faq_import_result":null,"created_at":"2026-04-24T15:36:28.354639+08:00","updated_at":"2026-04-24T15:36:30.038613+08:00","processed_at":null,"error_message":"","deleted_at":null,"knowledge_base_name":""}],"success":true} size=942 status_code=200]                      |
INFO [2026-04-24 15:36:45.534] [lJxYifTcYUfC] knowledge.go:977[GetKnowledgeBatch] | Batch retrieving knowledge without kb_id, effective tenant ID: 10000, IDs count: 1

2026/04/24 15:36:45 /home/xmkp/workspace/WeKnora/internal/application/repository/knowledge.go:182
[1.300ms] [rows:1] SELECT * FROM "knowledges" WHERE (tenant_id = 10000 AND id IN ('8bf9ea36-c2be-4c14-9dec-dc06a15a530b')) AND "knowledges"."deleted_at" IS NULL
INFO [2026-04-24 15:36:45.537] [lJxYifTcYUfC] knowledge.go:1011[GetKnowledgeBatch] | Batch knowledge retrieval successful, requested count: 1, returned count: 1
INFO [2026-04-24 15:36:45.539] [lJxYifTcYUfC client_ip=127.0.0.1 latency=27.724403ms method=GET path=/api/v1/knowledge/batch?ids=8bf9ea36-c2be-4c14-9dec-dc06a15a530b& response_body={"data":[{"id":"8bf9ea36-c2be-4c14-9dec-dc06a15a530b","tenant_id":10000,"knowledge_base_id":"bfeed109-3fcf-4cbc-8ce4-4291fedc40b1","tag_id":"","type":"file","title":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","description":"","source":"","channel":"web","parse_status":"processing","summary_status":"none","enable_status":"disabled","embedding_model_id":"afa200dd-d0f3-460d-999b-c3e8a9e7d554","file_name":"20260420-01-AI_OA_PC端与H5页面功能清单-精简AI.pdf","file_type":"pdf","file_size":5365539,"file_hash":"430dc0916b5caa5fcfa1841d6ee761f7","file_path":"minio://weknora/10000/8bf9ea36-c2be-4c14-9dec-dc06a15a530b/a2a8d175-9609-461d-ba19-c3620e53dc69.pdf","storage_size":0,"metadata":null,"last_faq_import_result":null,"created_at":"2026-04-24T15:36:28.354639+08:00","updated_at":"2026-04-24T15:36:30.038613+08:00","processed_at":null,"error_message":"","deleted_at":null,"knowledge_base_name":""}],"success":true} size=942 status_code=200]                      |

2026/04/24 15:36:49 /home/xmkp/workspace/WeKnora/internal/application/repository/user.go:162 SLOW SQL >= 200ms
[1439.905ms] [rows:1] SELECT * FROM "auth_tokens" WHERE token = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6ImFkbWluQGhsc2EuY29tIiwiZXhwIjoxNzc3MTAxMjc2LCJpYXQiOjE3NzcwMTQ4NzYsInRlbmFudF9pZCI6MTAwMDAsInR5cGUiOiJhY2Nlc3MiLCJ1c2VyX2lkIjoiNjM5NTJkMzMtMmM1Ny00OWZlLThhZjYtNTkzNjc0YmRiY2Q3In0.QERWy_LGpy3YAdcXwiWxIwMGFmmoUpOfyfSAc1-ZhOY' ORDER BY "auth_tokens"."id" LIMIT 1
asynq: pid=78581 2026/04/24 07:36:51.241788 ERROR: Failed to write server state data: UNKNOWN: redis command error: SADD failed: i/o timeout
asynq: pid=78581 2026/04/24 07:36:51.583028 ERROR: Dequeue error: UNKNOWN: redis eval error: i/o timeout
asynq: pid=78581 2026/04/24 07:36:51.667670 ERROR: Failed to forward scheduled tasks: INTERNAL_ERROR: INTERNAL_ERROR: redis eval error: i/o timeout
asynq: pid=78581 2026/04/24 07:36:56.496471 ERROR: Failed to extend lease for tasks [769eb3c8-ab89-4d1e-a9ee-7f4c84098145]: i/o timeout
redis: 2026/04/24 15:36:59 pubsub.go:171: redis: discarding bad PubSub connection: write tcp 127.0.0.1:47674->127.0.0.1:6379: i/o timeout
asynq: pid=78581 2026/04/24 07:36:59.740089 ERROR: Dequeue error: UNKNOWN: redis eval error: i/o timeout
asynq: pid=78581 2026/04/24 07:36:59.409732 ERROR: Failed to delete expired completed tasks from queue "low": INTERNAL_ERROR: redis eval error: i/o timeout
asynq: pid=78581 2026/04/24 07:37:07.650297 ERROR: Failed to forward scheduled tasks: INTERNAL_ERROR: INTERNAL_ERROR: redis eval error: dial tcp: lookup localhost: i/o timeout
asynq: pid=78581 2026/04/24 07:37:08.227721 ERROR: Failed to delete expired completed tasks from queue "critical": INTERNAL_ERROR: redis eval error: i/o timeout
^Casynq: pid=78581 2026/04/24 07:37:14.266673 INFO: Stopping processor
asynq: pid=78581 2026/04/24 07:37:19.683814 ERROR: Failed to write server state data: UNKNOWN: redis command error: SADD failed: dial tcp: lookup localhost: i/o timeout
asynq: pid=78581 2026/04/24 07:37:20.259236 ERROR: Dequeue error: UNKNOWN: redis eval error: i/o timeout
asynq: pid=78581 2026/04/24 07:37:21.322217 INFO: Processor stopped
asynq: pid=78581 2026/04/24 07:37:22.728210 INFO: Starting graceful shutdown
asynq: pid=78581 2026/04/24 07:37:24.972007 ERROR: Failed to delete expired completed tasks from queue "default": INTERNAL_ERROR: redis eval error: dial tcp 127.0.0.1:6379: i/o timeout
INFO [2026-04-24 15:37:23.370] [] main.go:91[2] | Received signal: interrupt, starting server shutdown...
asynq: pid=78581 2026/04/24 07:37:32.746773 ERROR: Failed to extend lease for tasks [769eb3c8-ab89-4d1e-a9ee-7f4c84098145]: dial tcp 127.0.0.1:6379: i/o timeout
asynq: pid=78581 2026/04/24 07:37:35.992888 ERROR: Failed to forward scheduled tasks: INTERNAL_ERROR: INTERNAL_ERROR: redis eval error: dial tcp: lookup localhost: i/o timeout
asynq: pid=78581 2026/04/24 07:37:39.701462 INFO: Waiting for all workers to finish...
FATAL[2026-04-24 15:37:41.076] [] main.go:137[main] | Failed to run application: server error: accept tcp [::]:8081: use of closed network connection
exit status 1

xmkp@hlsa:~/workspace/WeKnora$ make dev-app
./scripts/dev.sh app
[INFO] 启动后端应用（本地开发模式）...
[INFO] 加载 .env 文件...
[INFO] 环境变量已设置，启动应用...
[INFO] 数据库地址: localhost:5432
[INFO] 未检测到 Air，使用普通模式启动
[WARNING] 提示: 安装 Air 可以实现代码修改后自动重启
[INFO] 安装命令: go install github.com/air-verse/air@latest
WARNING: proto: file "common.proto" is already registered
        previously from: "github.com/milvus-io/milvus-proto/go-api/v2/commonpb"
        currently from:  "github.com/qdrant/go-client/qdrant"
See https://protobuf.dev/reference/go/faq#namespace-conflict

DEBUG[2026-04-24 15:37:58.882] [] container.go:94[BuildContainer] | [Container] Starting container initialization...
DEBUG[2026-04-24 15:37:58.882] [] container.go:100[BuildContainer] | [Container] Registering core infrastructure...
Using configuration file: /home/xmkp/workspace/WeKnora/config/config.yaml
DEBUG[2026-04-24 15:37:58.888] [] container.go:116[BuildContainer] | [Container] Registering retrieval engine registry...
DEBUG[2026-04-24 15:37:58.888] [] container.go:120[BuildContainer] | [Container] Registering external service clients...
DEBUG[2026-04-24 15:37:58.888] [] container.go:126[BuildContainer] | [Container] Initializing DuckDB...
DEBUG[2026-04-24 15:37:58.888] [] container.go:128[BuildContainer] | [Container] DuckDB registered
DEBUG[2026-04-24 15:37:58.888] [] container.go:131[BuildContainer] | [Container] Registering repositories...
DEBUG[2026-04-24 15:37:58.888] [] container.go:157[BuildContainer] | [Container] Registering MCP manager...
DEBUG[2026-04-24 15:37:58.888] [] container.go:161[BuildContainer] | [Container] Registering business services...
DEBUG[2026-04-24 15:37:58.889] [] container.go:195[BuildContainer] | [Container] Registering web search registry and providers...
DEBUG[2026-04-24 15:37:58.889] [] container.go:216[BuildContainer] | [Container] Registering event bus and agent service...
DEBUG[2026-04-24 15:37:58.889] [] container.go:222[BuildContainer] | [Container] Registering session service...
DEBUG[2026-04-24 15:37:58.889] [] container.go:225[BuildContainer] | [Container] Registering task enqueuer...
DEBUG[2026-04-24 15:37:58.889] [] container.go:237[BuildContainer] | [Container] Registering chat pipeline plugins...
DEBUG[2026-04-24 15:37:58.889] [] container.go:240[BuildContainer] | [Container] Registering data source sync framework...
INFO [2026-04-24 15:37:58.889] [] container.go:413[initDatabase] | Skip embedding: false
INFO [2026-04-24 15:37:58.889] [] container.go:426[initDatabase] | DB Config: user=postgres host=localhost port=5432 dbname=WeKnora
INFO [2026-04-24 15:37:58.898] [] container.go:474[initDatabase] | Running database migrations...
INFO [2026-04-24 15:37:58.898] [] migration.go:62[RunMigrationsWithOptions] | Starting database migration...
INFO [2026-04-24 15:37:58.923] [] migration.go:107[RunMigrationsWithOptions] | Current migration version: 41, dirty: false
INFO [2026-04-24 15:37:58.923] [] migration.go:144[RunMigrationsWithOptions] | Running pending migrations...
INFO [2026-04-24 15:37:58.925] [] migration.go:200[RunMigrationsWithOptions] | Database is up to date (version: 41)
INFO [2026-04-24 15:37:58.931] [] container.go:571[syncSequences] | Synced sequence chunks_seq_id_seq with table chunks
INFO [2026-04-24 15:37:58.934] [] container.go:571[syncSequences] | Synced sequence knowledge_tags_seq_id_seq with table knowledge_tags
INFO [2026-04-24 15:37:58.936] [] scheduler.go:72[Start] | [Scheduler] started with 0 cron entries
DEBUG[2026-04-24 15:37:58.936] [] container.go:245[BuildContainer] | [Container] Data source sync framework registered
INFO [2026-04-24 15:37:58.937] []                      | Ollama base URL: http://host.docker.internal:11434
INFO [2026-04-24 15:37:58.937] []                      | [Postgres] Initializing PostgreSQL retriever engine repository
INFO [2026-04-24 15:37:58.937] []                      | Register postgres retrieve engine success
panic: could not build arguments for function "github.com/Tencent/WeKnora/internal/application/service/chat_pipeline".NewPluginSearch (/home/xmkp/workspace/WeKnora/internal/application/service/chat_pipeline/search.go:29): failed to build interfaces.KnowledgeBaseService: could not build arguments for function "github.com/Tencent/WeKnora/internal/application/service".NewKnowledgeBaseService (/home/xmkp/workspace/WeKnora/internal/application/service/knowledgebase.go:36): failed to build interfaces.FileService: received non-nil error from function "github.com/Tencent/WeKnora/internal/container".initFileService (/home/xmkp/workspace/WeKnora/internal/container/container.go:632): failed to check bucket: Get "http://localhost:9000/weknora/?location=": dial tcp 127.0.0.1:9000: i/o timeout

goroutine 1 [running]:
github.com/Tencent/WeKnora/internal/container.must(...)
        /home/xmkp/workspace/WeKnora/internal/container/container.go:321
github.com/Tencent/WeKnora/internal/container.BuildContainer(0xc0001a8628)
        /home/xmkp/workspace/WeKnora/internal/container/container.go:247 +0x2277
main.main()
        /home/xmkp/workspace/WeKnora/cmd/server/main.go:52 +0x72
exit status 2