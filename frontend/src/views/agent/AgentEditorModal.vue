<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="visible" class="settings-overlay" @click.self="handleClose">
        <div class="settings-modal">
          <!-- 关闭按钮 -->
          <button class="close-btn" @click="handleClose" :aria-label="$t('common.close')">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
              <path d="M15 5L5 15M5 5L15 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
            </svg>
          </button>

          <div class="settings-container">
            <!-- 左侧导航 -->
            <div class="settings-sidebar">
              <div class="sidebar-header">
                <h2 class="sidebar-title">{{ mode === 'create' ? $t('agent.editor.createTitle') : $t('agent.editor.editTitle') }}</h2>
              </div>
              <div class="settings-nav">
                <div 
                  v-for="(item, index) in navItems" 
                  :key="index"
                  :class="['nav-item', { 'active': currentSection === item.key }]"
                  @click="currentSection = item.key"
                >
                  <t-icon :name="item.icon" class="nav-icon" />
                  <span class="nav-label">{{ item.label }}</span>
                </div>
              </div>
            </div>

            <!-- 右侧内容区域 -->
            <div class="settings-content">
              <div class="content-wrapper">
                <!-- 基础设置 -->
                <div v-show="currentSection === 'basic'" class="section">
                  <div class="section-header">
                    <h2>{{ $t('agent.editor.basicInfo') }}</h2>
                    <p class="section-description">{{ $t('agent.editor.basicInfoDesc') || '配置智能体的基本信息' }}</p>
                  </div>
                  
                  <div class="settings-group">
                    <!-- 内置智能体提示 -->
                    <div v-if="isBuiltinAgent" class="builtin-agent-notice">
                      <t-icon name="info-circle" />
                      <span>这是内置智能体，名称和描述不可修改，但可以调整配置参数</span>
                    </div>

                    <!-- 运行模式（首先选择） -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.mode') }} <span class="required">*</span></label>
                        <p class="desc">{{ agentMode === 'agent' ? $t('agent.editor.agentDesc') : $t('agent.editor.normalDesc') }}</p>
                      </div>
                      <div class="setting-control">
                        <t-radio-group v-model="agentMode" :disabled="isBuiltinAgent">
                          <t-radio-button value="normal">
                            {{ $t('agent.type.normal') }}
                          </t-radio-button>
                          <t-radio-button value="agent">
                            {{ $t('agent.type.agent') }}
                          </t-radio-button>
                        </t-radio-group>
                      </div>
                    </div>

                    <!-- 名称 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.name') }} <span v-if="!isBuiltinAgent" class="required">*</span></label>
                        <p class="desc">为智能体设置一个易于识别的名称</p>
                      </div>
                      <div class="setting-control">
                        <div class="name-input-wrapper">
                          <AgentAvatar :name="formData.name || '?'" size="large" />
                          <t-input 
                            v-model="formData.name" 
                            :placeholder="$t('agent.editor.namePlaceholder')" 
                            class="name-input"
                            :disabled="isBuiltinAgent"
                          />
                        </div>
                      </div>
                    </div>

                    <!-- 描述 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.description') }}</label>
                        <p class="desc">简要描述智能体的用途和特点</p>
                      </div>
                      <div class="setting-control">
                        <t-textarea 
                          v-model="formData.description" 
                          :placeholder="$t('agent.editor.descriptionPlaceholder')"
                          :autosize="{ minRows: 2, maxRows: 4 }"
                          :disabled="isBuiltinAgent"
                        />
                      </div>
                    </div>

                    <!-- 系统提示词 -->
                    <div class="setting-row setting-row-vertical">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.systemPrompt') }} <span v-if="!isBuiltinAgent" class="required">*</span></label>
                        <p class="desc">自定义系统提示词，定义智能体的行为和角色{{ isBuiltinAgent ? '（留空则使用系统默认）' : '' }}</p>
                        <div class="placeholder-hint">
                          <p class="hint-title">{{ $t('agent.editor.availablePlaceholders') }}</p>
                          <ul class="placeholder-list">
                            <li v-for="placeholder in availablePlaceholders" :key="placeholder.name">
                              <code v-html="`{{${placeholder.name}}}`"></code> - {{ placeholder.description }}
                            </li>
                          </ul>
                          <p class="hint-tip">{{ $t('agent.editor.placeholderHint') }}</p>
                        </div>
                      </div>
                      <div class="setting-control setting-control-full" style="position: relative;">
                        <!-- Agent模式：网络搜索开/关两个提示词 -->
                        <div v-if="isAgentMode" class="system-prompt-tabs">
                          <t-tabs v-model="activePromptTab" class="prompt-variant-tabs">
                            <t-tab-panel value="web-disabled" label="网络搜索关闭">
                              <div class="textarea-with-template">
                                <t-textarea 
                                  ref="promptTextareaRef"
                                  v-model="formData.config.system_prompt" 
                                  :placeholder="systemPromptPlaceholder"
                                  :autosize="{ minRows: 10, maxRows: 25 }"
                                  @input="handlePromptInput"
                                  class="system-prompt-textarea"
                                />
                                <PromptTemplateSelector 
                                  type="systemPrompt" 
                                  position="corner"
                                  :hasKnowledgeBase="hasKnowledgeBase"
                                  @select="handleSystemPromptTemplateSelect"
                                />
                              </div>
                            </t-tab-panel>
                            <t-tab-panel value="web-enabled" label="网络搜索开启">
                              <div class="textarea-with-template">
                                <t-textarea 
                                  v-model="formData.config.system_prompt_web_enabled" 
                                  :placeholder="$t('agent.editor.systemPromptWebEnabledPlaceholder') || '网络搜索开启时使用的系统提示词'"
                                  :autosize="{ minRows: 10, maxRows: 25 }"
                                  class="system-prompt-textarea"
                                />
                                <PromptTemplateSelector 
                                  type="systemPrompt" 
                                  position="corner"
                                  :hasKnowledgeBase="hasKnowledgeBase"
                                  @select="handleSystemPromptTemplateSelect"
                                />
                              </div>
                            </t-tab-panel>
                          </t-tabs>
                        </div>
                        <!-- 普通模式：单个提示词 -->
                        <div v-else class="textarea-with-template">
                          <t-textarea 
                            ref="promptTextareaRef"
                            v-model="formData.config.system_prompt" 
                            :placeholder="systemPromptPlaceholder"
                            :autosize="{ minRows: 10, maxRows: 25 }"
                            @input="handlePromptInput"
                            class="system-prompt-textarea"
                          />
                          <PromptTemplateSelector 
                            type="systemPrompt" 
                            position="corner"
                            :hasKnowledgeBase="hasKnowledgeBase"
                            @select="handleSystemPromptTemplateSelect"
                          />
                        </div>
                        <!-- 占位符提示下拉框 -->
                        <Teleport to="body">
                          <div
                            v-if="showPlaceholderPopup && filteredPlaceholders.length > 0"
                            class="placeholder-popup-wrapper"
                            :style="popupStyle"
                          >
                            <div class="placeholder-popup">
                              <div
                                v-for="(placeholder, index) in filteredPlaceholders"
                                :key="placeholder.name"
                                class="placeholder-item"
                                :class="{ active: selectedPlaceholderIndex === index }"
                                @mousedown.prevent="insertPlaceholder(placeholder.name)"
                                @mouseenter="selectedPlaceholderIndex = index"
                              >
                                <div class="placeholder-name">
                                  <code v-html="`{{${placeholder.name}}}`"></code>
                                </div>
                                <div class="placeholder-desc">{{ placeholder.description }}</div>
                              </div>
                            </div>
                          </div>
                        </Teleport>
                      </div>
                    </div>

                    <!-- 上下文模板（仅普通模式） -->
                    <div v-if="!isAgentMode" class="setting-row setting-row-vertical">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.contextTemplate') || '上下文模板' }} <span v-if="!isBuiltinAgent" class="required">*</span></label>
                        <p class="desc">定义如何将检索到的内容格式化后传递给模型{{ isBuiltinAgent ? '（留空则使用系统默认）' : '' }}</p>
                        <div class="placeholder-hint">
                          <p class="hint-title">{{ $t('agent.editor.availableContextPlaceholders') || '可用占位符' }}</p>
                          <ul class="placeholder-list">
                            <li v-for="placeholder in contextTemplatePlaceholders" :key="placeholder.name">
                              <code v-html="`{{${placeholder.name}}}`"></code> - {{ placeholder.description }}
                            </li>
                          </ul>
                        </div>
                      </div>
                      <div class="setting-control setting-control-full" style="position: relative;">
                        <div class="textarea-with-template">
                          <t-textarea 
                            ref="contextTemplateTextareaRef"
                            v-model="formData.config.context_template" 
                            :placeholder="contextTemplatePlaceholder"
                            :autosize="{ minRows: 8, maxRows: 20 }"
                            @input="handleContextTemplateInput"
                            class="system-prompt-textarea"
                          />
                          <PromptTemplateSelector 
                            type="contextTemplate" 
                            position="corner"
                            :hasKnowledgeBase="hasKnowledgeBase"
                            @select="handleContextTemplateSelect"
                          />
                        </div>
                        <!-- 上下文模板占位符提示下拉框 -->
                        <Teleport to="body">
                          <div
                            v-if="showContextPlaceholderPopup && filteredContextPlaceholders.length > 0"
                            class="placeholder-popup-wrapper"
                            :style="contextPopupStyle"
                          >
                            <div class="placeholder-popup">
                              <div
                                v-for="(placeholder, index) in filteredContextPlaceholders"
                                :key="placeholder.name"
                                class="placeholder-item"
                                :class="{ active: selectedContextPlaceholderIndex === index }"
                                @mousedown.prevent="insertContextPlaceholder(placeholder.name)"
                                @mouseenter="selectedContextPlaceholderIndex = index"
                              >
                                <div class="placeholder-name">
                                  <code v-html="`{{${placeholder.name}}}`"></code>
                                </div>
                                <div class="placeholder-desc">{{ placeholder.description }}</div>
                              </div>
                            </div>
                          </div>
                        </Teleport>
                      </div>
                    </div>

                  </div>
                </div>

                <!-- 模型配置 -->
                <div v-show="currentSection === 'model'" class="section">
                  <div class="section-header">
                    <h2>{{ $t('agent.editor.modelConfig') }}</h2>
                    <p class="section-description">{{ $t('agent.editor.modelConfigDesc') || '配置智能体的模型参数' }}</p>
                  </div>
                  
                  <div class="settings-group">
                    <!-- 模型选择 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.model') }} <span class="required">*</span></label>
                        <p class="desc">选择智能体使用的大语言模型</p>
                      </div>
                      <div class="setting-control">
                        <t-select v-model="formData.config.model_id" :placeholder="$t('agent.editor.modelPlaceholder')" filterable>
                          <t-option 
                            v-for="model in modelOptions" 
                            :key="model.value" 
                            :value="model.value" 
                            :label="model.label"
                          />
                        </t-select>
                      </div>
                    </div>

                    <!-- 温度 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.temperature') }}</label>
                        <p class="desc">控制输出的随机性，0 最确定，1 最随机</p>
                      </div>
                      <div class="setting-control">
                        <div class="slider-wrapper">
                          <t-slider v-model="formData.config.temperature" :min="0" :max="1" :step="0.1" />
                          <span class="slider-value">{{ formData.config.temperature }}</span>
                        </div>
                      </div>
                    </div>

                    <!-- 最大生成Token数 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.maxCompletionTokens') || '最大生成Token数' }}</label>
                        <p class="desc">模型生成回复的最大Token数量</p>
                      </div>
                      <div class="setting-control">
                        <t-input-number v-model="formData.config.max_completion_tokens" :min="100" :max="100000" :step="100" theme="column" />
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 多轮对话（仅普通模式显示，Agent模式内部自动控制） -->
                <div v-show="currentSection === 'conversation' && !isAgentMode" class="section">
                  <div class="section-header">
                    <h2>{{ $t('agent.editor.conversationSettings') || '多轮对话' }}</h2>
                    <p class="section-description">配置多轮对话和问题改写相关参数</p>
                  </div>
                  
                  <div class="settings-group">
                    <!-- 多轮对话 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.multiTurn') }}</label>
                        <p class="desc">开启后将保留历史对话上下文</p>
                      </div>
                      <div class="setting-control">
                        <t-switch v-model="formData.config.multi_turn_enabled" />
                      </div>
                    </div>

                    <!-- 保留轮数 -->
                    <div v-if="formData.config.multi_turn_enabled" class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.historyTurns') }}</label>
                        <p class="desc">保留最近几轮对话作为上下文</p>
                      </div>
                      <div class="setting-control">
                        <t-input-number v-model="formData.config.history_turns" :min="1" :max="20" theme="column" />
                      </div>
                    </div>

                    <!-- 问题改写（仅多轮对话开启且普通模式时显示） -->
                    <div v-if="formData.config.multi_turn_enabled && !isAgentMode" class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.enableRewrite') || '问题改写' }}</label>
                        <p class="desc">多轮对话时自动改写用户问题，消解指代和补全省略</p>
                      </div>
                      <div class="setting-control">
                        <t-switch v-model="formData.config.enable_rewrite" />
                      </div>
                    </div>

                    <!-- 改写系统提示词 -->
                    <div v-if="formData.config.multi_turn_enabled && !isAgentMode && formData.config.enable_rewrite" class="setting-row setting-row-vertical">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.rewritePromptSystem') || '改写系统提示词' }}</label>
                        <p class="desc">用于问题改写的系统提示词（留空使用默认）</p>
                      </div>
                      <div class="setting-control setting-control-full">
                        <div class="textarea-with-template">
                          <t-textarea 
                            v-model="formData.config.rewrite_prompt_system" 
                            :placeholder="defaultRewritePromptSystem || $t('agent.editor.rewritePromptSystemPlaceholder') || '留空使用系统默认提示词'"
                            :autosize="{ minRows: 4, maxRows: 10 }"
                          />
                          <PromptTemplateSelector 
                            type="rewriteSystem" 
                            position="corner"
                            @select="handleRewriteSystemTemplateSelect"
                          />
                        </div>
                      </div>
                    </div>

                    <!-- 改写用户提示词 -->
                    <div v-if="formData.config.multi_turn_enabled && !isAgentMode && formData.config.enable_rewrite" class="setting-row setting-row-vertical">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.rewritePromptUser') || '改写用户提示词' }}</label>
                        <p class="desc">用于问题改写的用户提示词模板（留空使用默认）</p>
                      </div>
                      <div class="setting-control setting-control-full">
                        <div class="textarea-with-template">
                          <t-textarea 
                            v-model="formData.config.rewrite_prompt_user" 
                            :placeholder="defaultRewritePromptUser || $t('agent.editor.rewritePromptUserPlaceholder') || '留空使用系统默认提示词'"
                            :autosize="{ minRows: 4, maxRows: 10 }"
                          />
                          <PromptTemplateSelector 
                            type="rewriteUser" 
                            position="corner"
                            @select="handleRewriteUserTemplateSelect"
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 工具配置（仅 Agent 模式） -->
                <div v-show="currentSection === 'tools' && isAgentMode" class="section">
                  <div class="section-header">
                    <h2>{{ $t('agent.editor.toolsConfig') || '工具配置' }}</h2>
                    <p class="section-description">{{ $t('agent.editor.toolsConfigDesc') || '配置 Agent 可以使用的工具' }}</p>
                  </div>
                  
                  <div class="settings-group">
                    <!-- 允许的工具 -->
                    <div class="setting-row setting-row-vertical">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.allowedTools') }}</label>
                        <p class="desc">选择 Agent 可以使用的工具</p>
                      </div>
                      <div class="setting-control setting-control-full">
                        <t-checkbox-group v-model="formData.config.allowed_tools" class="tools-checkbox-group">
                          <t-checkbox 
                            v-for="tool in availableTools" 
                            :key="tool.value" 
                            :value="tool.value"
                            :disabled="tool.disabled"
                            :class="['tool-checkbox-item', { 'tool-disabled': tool.disabled }]"
                          >
                            <div class="tool-item-content">
                              <span class="tool-name">{{ tool.label }}</span>
                              <span v-if="tool.description" class="tool-desc">{{ tool.description }}</span>
                              <span v-if="tool.disabled" class="tool-disabled-hint">（需要配置知识库）</span>
                            </div>
                          </t-checkbox>
                        </t-checkbox-group>
                      </div>
                    </div>

                    <!-- 最大迭代次数 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.maxIterations') }}</label>
                        <p class="desc">Agent 执行任务时的最大推理步骤数</p>
                      </div>
                      <div class="setting-control">
                        <t-input-number v-model="formData.config.max_iterations" :min="1" :max="50" theme="column" />
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 知识库配置 -->
                <div v-show="currentSection === 'knowledge'" class="section">
                  <div class="section-header">
                    <h2>{{ $t('agent.editor.knowledgeConfig') || '知识库' }}</h2>
                    <p class="section-description">{{ $t('agent.editor.knowledgeConfigDesc') || '配置智能体可访问的知识库' }}</p>
                  </div>
                  
                  <div class="settings-group">
                    <!-- 关联知识库 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.knowledgeBases') }}</label>
                        <p class="desc">选择智能体可访问的知识库范围</p>
                      </div>
                      <div class="setting-control">
                        <t-radio-group v-model="kbSelectionMode">
                          <t-radio-button value="all">{{ $t('agent.editor.allKnowledgeBases') || '全部' }}</t-radio-button>
                          <t-radio-button value="selected">{{ $t('agent.editor.selectedKnowledgeBases') || '指定' }}</t-radio-button>
                          <t-radio-button value="none">{{ $t('agent.editor.noKnowledgeBase') || '禁用' }}</t-radio-button>
                        </t-radio-group>
                      </div>
                    </div>

                    <!-- 选择指定知识库（仅在选择"指定知识库"时显示） -->
                    <div v-if="kbSelectionMode === 'selected'" class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.selectKnowledgeBases') }}</label>
                        <p class="desc">选择要关联的知识库</p>
                      </div>
                      <div class="setting-control">
                        <t-select 
                          v-model="formData.config.knowledge_bases" 
                          multiple 
                          :placeholder="$t('agent.editor.selectKnowledgeBases')"
                          filterable
                        >
                          <t-option 
                            v-for="kb in kbOptions" 
                            :key="kb.value" 
                            :value="kb.value" 
                            :label="kb.label" 
                          />
                        </t-select>
                      </div>
                    </div>

                    <!-- 允许用户选择知识库（有知识库能力时显示） -->
                    <div v-if="kbSelectionMode !== 'none'" class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.allowUserKBSelection') }}</label>
                        <p class="desc">{{ allowUserKBSelectionDesc }}</p>
                      </div>
                      <div class="setting-control">
                        <t-switch v-model="formData.config.allow_user_kb_selection" />
                      </div>
                    </div>

                    <!-- ReRank 模型（当配置了知识库或允许用户选择知识库时显示） -->
                    <div v-if="needsRerankModel" class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.rerankModel') }} <span class="required">*</span></label>
                        <p class="desc">{{ $t('agent.editor.rerankModelDesc') }}</p>
                      </div>
                      <div class="setting-control">
                        <t-select 
                          v-model="formData.config.rerank_model_id" 
                          :placeholder="$t('agent.editor.rerankModelPlaceholder')"
                          filterable
                        >
                          <t-option 
                            v-for="model in rerankModelOptions" 
                            :key="model.value" 
                            :value="model.value" 
                            :label="model.label"
                          />
                        </t-select>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 网络搜索配置 -->
                <div v-show="currentSection === 'websearch'" class="section">
                  <div class="section-header">
                    <h2>{{ $t('agent.editor.webSearchConfig') || '网络搜索' }}</h2>
                    <p class="section-description">{{ $t('agent.editor.webSearchConfigDesc') || '配置智能体的网络搜索能力' }}</p>
                  </div>
                  
                  <div class="settings-group">
                    <!-- 网络搜索 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.webSearch') }}</label>
                        <p class="desc">启用后智能体可以搜索互联网获取信息</p>
                      </div>
                      <div class="setting-control">
                        <t-switch v-model="formData.config.web_search_enabled" />
                      </div>
                    </div>

                    <!-- 网络搜索最大结果数 -->
                    <div v-if="formData.config.web_search_enabled" class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.webSearchMaxResults') }}</label>
                        <p class="desc">每次搜索返回的最大结果数量</p>
                      </div>
                      <div class="setting-control">
                        <div class="slider-wrapper">
                          <t-slider v-model="formData.config.web_search_max_results" :min="1" :max="10" />
                          <span class="slider-value">{{ formData.config.web_search_max_results }}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 检索策略（仅在有知识库能力时显示） -->
                <div v-show="currentSection === 'retrieval' && hasKnowledgeBase" class="section">
                  <div class="section-header">
                    <h2>{{ $t('agent.editor.retrievalStrategy') || '检索策略' }}</h2>
                    <p class="section-description">配置知识库检索和排序的参数</p>
                  </div>
                  
                  <div class="settings-group">
                    <!-- 查询扩展（仅普通模式） -->
                    <div v-if="!isAgentMode" class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.enableQueryExpansion') || '查询扩展' }}</label>
                        <p class="desc">自动扩展查询词以提高召回率</p>
                      </div>
                      <div class="setting-control">
                        <t-switch v-model="formData.config.enable_query_expansion" />
                      </div>
                    </div>

                    <!-- 向量召回TopK -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.embeddingTopK') || '向量召回数量' }}</label>
                        <p class="desc">向量检索返回的最大结果数量</p>
                      </div>
                      <div class="setting-control">
                        <t-input-number v-model="formData.config.embedding_top_k" :min="1" :max="50" theme="column" />
                      </div>
                    </div>

                    <!-- 关键词阈值 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.keywordThreshold') || '关键词阈值' }}</label>
                        <p class="desc">关键词检索的最低相关性分数</p>
                      </div>
                      <div class="setting-control">
                        <div class="slider-wrapper">
                          <t-slider v-model="formData.config.keyword_threshold" :min="0" :max="1" :step="0.05" />
                          <span class="slider-value">{{ formData.config.keyword_threshold?.toFixed(2) }}</span>
                        </div>
                      </div>
                    </div>

                    <!-- 向量阈值 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.vectorThreshold') || '向量阈值' }}</label>
                        <p class="desc">向量检索的最低相似度分数</p>
                      </div>
                      <div class="setting-control">
                        <div class="slider-wrapper">
                          <t-slider v-model="formData.config.vector_threshold" :min="0" :max="1" :step="0.05" />
                          <span class="slider-value">{{ formData.config.vector_threshold?.toFixed(2) }}</span>
                        </div>
                      </div>
                    </div>

                    <!-- 重排TopK -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.rerankTopK') || '重排数量' }}</label>
                        <p class="desc">重排序后保留的最大结果数量</p>
                      </div>
                      <div class="setting-control">
                        <t-input-number v-model="formData.config.rerank_top_k" :min="1" :max="20" theme="column" />
                      </div>
                    </div>

                    <!-- 重排阈值 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.rerankThreshold') || '重排阈值' }}</label>
                        <p class="desc">重排序的最低相关性分数</p>
                      </div>
                      <div class="setting-control">
                        <div class="slider-wrapper">
                          <t-slider v-model="formData.config.rerank_threshold" :min="0" :max="1" :step="0.05" />
                          <span class="slider-value">{{ formData.config.rerank_threshold?.toFixed(2) }}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- 高级设置（仅普通模式） -->
                <div v-show="currentSection === 'advanced' && !isAgentMode" class="section">
                  <div class="section-header">
                    <h2>{{ $t('agent.editor.advancedSettings') || '高级设置' }}</h2>
                    <p class="section-description">配置兜底策略</p>
                  </div>
                  
                  <div class="settings-group">
                    <!-- 兜底策略 -->
                    <div class="setting-row">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.fallbackStrategy') || '兜底策略' }}</label>
                        <p class="desc">当无法从知识库找到相关内容时的处理方式</p>
                      </div>
                      <div class="setting-control">
                        <t-radio-group v-model="formData.config.fallback_strategy">
                          <t-radio-button value="fixed">固定回复</t-radio-button>
                          <t-radio-button value="model">模型生成</t-radio-button>
                        </t-radio-group>
                      </div>
                    </div>

                    <!-- 固定兜底回复 -->
                    <div v-if="formData.config.fallback_strategy === 'fixed'" class="setting-row setting-row-vertical">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.fallbackResponse') || '固定回复内容' }}</label>
                        <p class="desc">当无法回答时返回的固定文本</p>
                      </div>
                      <div class="setting-control setting-control-full">
                        <div class="textarea-with-template">
                          <t-textarea 
                            v-model="formData.config.fallback_response" 
                            :placeholder="defaultFallbackResponse || $t('agent.editor.fallbackResponsePlaceholder') || '抱歉，我无法回答这个问题。'"
                            :autosize="{ minRows: 2, maxRows: 6 }"
                          />
                          <PromptTemplateSelector 
                            type="fallback" 
                            position="corner"
                            @select="handleFallbackResponseTemplateSelect"
                          />
                        </div>
                      </div>
                    </div>

                    <!-- 兜底提示词 -->
                    <div v-if="formData.config.fallback_strategy === 'model'" class="setting-row setting-row-vertical">
                      <div class="setting-info">
                        <label>{{ $t('agent.editor.fallbackPrompt') || '兜底提示词' }}</label>
                        <p class="desc">当无法从知识库找到答案时，引导模型生成回复的提示词</p>
                      </div>
                      <div class="setting-control setting-control-full">
                        <div class="textarea-with-template">
                          <t-textarea 
                            v-model="formData.config.fallback_prompt" 
                            :placeholder="defaultFallbackPrompt || $t('agent.editor.fallbackPromptPlaceholder') || '留空使用系统默认提示词'"
                            :autosize="{ minRows: 4, maxRows: 10 }"
                          />
                          <PromptTemplateSelector 
                            type="fallback" 
                            position="corner"
                            @select="handleFallbackPromptTemplateSelect"
                          />
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- 底部操作栏 -->
              <div class="settings-footer">
                <t-button variant="outline" @click="handleClose">{{ $t('common.cancel') }}</t-button>
                <t-button theme="primary" :loading="saving" @click="handleSave">{{ $t('common.confirm') }}</t-button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import { createAgent, updateAgent, type CustomAgent } from '@/api/agent';
import { listModels } from '@/api/model';
import { listKnowledgeBases } from '@/api/knowledge-base';
import { getAgentConfig, getConversationConfig, type PlaceholderDefinition } from '@/api/system';
import AgentAvatar from '@/components/AgentAvatar.vue';
import PromptTemplateSelector from '@/components/PromptTemplateSelector.vue';

const { t } = useI18n();

const props = defineProps<{
  visible: boolean;
  mode: 'create' | 'edit';
  agent?: CustomAgent | null;
}>();

const emit = defineEmits<{
  (e: 'update:visible', visible: boolean): void;
  (e: 'success'): void;
}>();

const currentSection = ref('basic');
const saving = ref(false);
const modelOptions = ref<{ label: string; value: string }[]>([]);
const rerankModelOptions = ref<{ label: string; value: string }[]>([]);
const kbOptions = ref<{ label: string; value: string }[]>([]);

// 系统默认配置（用于内置智能体显示默认提示词）
const defaultSystemPrompt = ref('');
const defaultContextTemplate = ref('');
const defaultRewritePromptSystem = ref('');
const defaultRewritePromptUser = ref('');
const defaultFallbackPrompt = ref('');
const defaultFallbackResponse = ref('');
// 默认检索参数
const defaultEmbeddingTopK = ref(10);
const defaultKeywordThreshold = ref(0.3);
const defaultVectorThreshold = ref(0.5);
const defaultRerankTopK = ref(5);
const defaultRerankThreshold = ref(0.5);
const defaultMaxCompletionTokens = ref(2048);
const defaultTemperature = ref(0.7);

// 知识库相关工具列表
const knowledgeBaseTools = ['grep_chunks', 'knowledge_search', 'list_knowledge_chunks', 'query_knowledge_graph', 'get_document_info', 'database_query'];

// 知识库选择模式：all=全部, selected=指定, none=不使用
const kbSelectionMode = ref<'all' | 'selected' | 'none'>('none');

// 可用工具列表 (与后台 definitions.go 保持一致)
const allTools = [
  { value: 'thinking', label: '思考', description: '动态和反思性的问题解决思考工具', requiresKB: false },
  { value: 'todo_write', label: '制定计划', description: '创建结构化的研究计划', requiresKB: false },
  { value: 'grep_chunks', label: '关键词搜索', description: '快速定位包含特定关键词的文档和分块', requiresKB: true },
  { value: 'knowledge_search', label: '语义搜索', description: '理解问题并查找语义相关内容', requiresKB: true },
  { value: 'list_knowledge_chunks', label: '查看文档分块', description: '获取文档完整分块内容', requiresKB: true },
  { value: 'query_knowledge_graph', label: '查询知识图谱', description: '从知识图谱中查询关系', requiresKB: true },
  { value: 'get_document_info', label: '获取文档信息', description: '查看文档元数据', requiresKB: true },
  { value: 'database_query', label: '查询数据库', description: '查询数据库中的信息', requiresKB: true },
];

// 根据知识库配置动态计算是否有知识库能力
const hasKnowledgeBase = computed(() => {
  return kbSelectionMode.value !== 'none';
});

// 允许用户选择知识库的描述
const allowUserKBSelectionDesc = computed(() => {
  if (kbSelectionMode.value === 'all') {
    return t('agent.editor.allowUserKBSelectionDescAll');
  }
  return t('agent.editor.allowUserKBSelectionDescSelected');
});

const availableTools = computed(() => {
  return allTools.map(tool => ({
    ...tool,
    disabled: tool.requiresKB && !hasKnowledgeBase.value
  }));
});

// 占位符相关
const availablePlaceholders = ref<PlaceholderDefinition[]>([]);
const promptTextareaRef = ref<any>(null);
const showPlaceholderPopup = ref(false);
const selectedPlaceholderIndex = ref(0);
const placeholderPrefix = ref('');
const popupStyle = ref({ top: '0px', left: '0px' });
let placeholderPopupTimer: any = null;

// 上下文模板占位符相关
const contextTemplateTextareaRef = ref<any>(null);
const showContextPlaceholderPopup = ref(false);
const selectedContextPlaceholderIndex = ref(0);
const contextPlaceholderPrefix = ref('');
const contextPopupStyle = ref({ top: '0px', left: '0px' });
let contextPlaceholderPopupTimer: any = null;

// 上下文模板可用的占位符
const contextTemplatePlaceholders = computed(() => [
  { name: 'query', description: t('agent.editor.placeholderQuery') || '用户的问题' },
  { name: 'contexts', description: t('agent.editor.placeholderContexts') || '检索到的内容列表' },
  { name: 'current_time', description: t('agent.editor.placeholderCurrentTime') || '当前时间（格式：2006-01-02 15:04:05）' },
  { name: 'current_week', description: t('agent.editor.placeholderCurrentWeek') || '当前星期（如：星期一）' },
]);

const navItems = computed(() => {
  const items: { key: string; icon: string; label: string }[] = [
    { key: 'basic', icon: 'info-circle', label: t('agent.editor.basicInfo') },
    { key: 'model', icon: 'control-platform', label: t('agent.editor.modelConfig') },
  ];
  // 知识库配置（放在工具上面）
  items.push({ key: 'knowledge', icon: 'folder', label: t('agent.editor.knowledgeConfig') || '知识库' });
  // Agent模式才显示工具配置
  if (isAgentMode.value) {
    items.push({ key: 'tools', icon: 'tools', label: t('agent.editor.toolsConfig') || '工具配置' });
  }
  // 有知识库能力时才显示检索策略
  if (hasKnowledgeBase.value) {
    items.push({ key: 'retrieval', icon: 'search', label: t('agent.editor.retrievalStrategy') || '检索策略' });
  }
  // 网络搜索（独立菜单）
  items.push({ key: 'websearch', icon: 'internet', label: t('agent.editor.webSearchConfig') || '网络搜索' });
  // 多轮对话（仅普通模式显示，Agent模式内部自动控制）
  if (!isAgentMode.value) {
    items.push({ key: 'conversation', icon: 'chat', label: t('agent.editor.conversationSettings') || '多轮对话' });
  }
  // 普通模式才显示高级设置
  if (!isAgentMode.value) {
    items.push({ key: 'advanced', icon: 'setting', label: t('agent.editor.advancedSettings') || '高级设置' });
  }
  return items;
});

// 初始数据
const defaultFormData = {
  name: '',
  description: '',
  type: 'custom' as const,
  is_builtin: false,
  config: {
    // 基础设置
    agent_mode: 'normal' as 'normal' | 'agent',
    system_prompt: '',
    system_prompt_web_enabled: '',
    context_template: '{{query}}',
    // 模型设置
    model_id: '',
    rerank_model_id: '',
    temperature: 0.7,
    max_completion_tokens: 2048,
    // Agent模式设置
    max_iterations: 10,
    allowed_tools: [] as string[],
    reflection_enabled: false,
    // 知识库设置
    kb_selection_mode: 'none' as 'all' | 'selected' | 'none',
    knowledge_bases: [] as string[],
    allow_user_kb_selection: false,
    // 网络搜索设置
    web_search_enabled: false,
    web_search_max_results: 5,
    // 多轮对话设置
    multi_turn_enabled: false,
    history_turns: 5,
    // 检索策略设置
    embedding_top_k: 10,
    keyword_threshold: 0.3,
    vector_threshold: 0.5,
    rerank_top_k: 5,
    rerank_threshold: 0.5,
    // 高级设置（普通模式）
    enable_query_expansion: true,
    enable_rewrite: true,
    rewrite_prompt_system: '',
    rewrite_prompt_user: '',
    fallback_strategy: 'model' as 'fixed' | 'model',
    fallback_response: '',
    fallback_prompt: '',
    // 已废弃字段（保留兼容）
    welcome_message: '',
    suggested_prompts: [] as string[],
  }
};

const formData = ref(JSON.parse(JSON.stringify(defaultFormData)));
const agentMode = computed({
  get: () => formData.value.config.agent_mode,
  set: (val: 'normal' | 'agent') => { formData.value.config.agent_mode = val; }
});

const isAgentMode = computed(() => agentMode.value === 'agent');

// 是否为内置智能体
const isBuiltinAgent = computed(() => {
  return formData.value.is_builtin === true;
});

// 系统提示词的 placeholder（内置智能体显示默认提示词，自定义智能体显示普通提示）
const systemPromptPlaceholder = computed(() => {
  if (isBuiltinAgent.value && defaultSystemPrompt.value) {
    return t('agent.editor.defaultPromptHint') + '\n\n' + defaultSystemPrompt.value;
  }
  return t('agent.editor.systemPromptPlaceholder');
});

// 上下文模板的 placeholder（内置智能体显示默认模板，自定义智能体显示普通提示）
const contextTemplatePlaceholder = computed(() => {
  if (isBuiltinAgent.value && defaultContextTemplate.value) {
    return t('agent.editor.defaultContextTemplateHint') + '\n\n' + defaultContextTemplate.value;
  }
  return t('agent.editor.contextTemplatePlaceholder');
});

// Agent模式系统提示词Tab
const activePromptTab = ref<'web-disabled' | 'web-enabled'>('web-disabled');

// 是否需要配置 ReRank 模型（有知识库能力时需要）
const needsRerankModel = computed(() => {
  return hasKnowledgeBase.value;
});

// 监听可见性变化，重置表单
watch(() => props.visible, async (val) => {
  if (val) {
    currentSection.value = 'basic';
    // 先加载依赖数据（包括默认配置）
    await loadDependencies();
    
    if (props.mode === 'edit' && props.agent) {
      // 深度复制对象以避免引用问题
      const agentData = JSON.parse(JSON.stringify(props.agent));
      
      // 确保 config 对象存在
      if (!agentData.config) {
        agentData.config = JSON.parse(JSON.stringify(defaultFormData.config));
      }
      
      // 补全可能缺失的字段
      agentData.config = { ...defaultFormData.config, ...agentData.config };
      
      // 确保数组字段存在
      if (!agentData.config.suggested_prompts) agentData.config.suggested_prompts = [];
      if (!agentData.config.knowledge_bases) agentData.config.knowledge_bases = [];
      if (!agentData.config.allowed_tools) agentData.config.allowed_tools = [];

      // 兼容旧数据：如果没有 agent_mode 字段，根据 allowed_tools 推断
      if (!agentData.config.agent_mode) {
        const isAgent = agentData.config.max_iterations > 1 || (agentData.config.allowed_tools && agentData.config.allowed_tools.length > 0);
        agentData.config.agent_mode = isAgent ? 'agent' : 'normal';
      }

      formData.value = agentData;
      // 初始化知识库选择模式
      initKbSelectionMode();
    } else {
      // 创建新智能体，使用系统默认值
      const newFormData = JSON.parse(JSON.stringify(defaultFormData));
      // 应用系统默认检索参数
      newFormData.config.embedding_top_k = defaultEmbeddingTopK.value;
      newFormData.config.keyword_threshold = defaultKeywordThreshold.value;
      newFormData.config.vector_threshold = defaultVectorThreshold.value;
      newFormData.config.rerank_top_k = defaultRerankTopK.value;
      newFormData.config.rerank_threshold = defaultRerankThreshold.value;
      newFormData.config.max_completion_tokens = defaultMaxCompletionTokens.value;
      newFormData.config.temperature = defaultTemperature.value;
      // 应用系统默认上下文模板
      if (defaultContextTemplate.value) {
        newFormData.config.context_template = defaultContextTemplate.value;
      }
      formData.value = newFormData;
      kbSelectionMode.value = 'none';
    }
  }
});

// 初始化知识库选择模式
const initKbSelectionMode = () => {
  if (formData.value.config.kb_selection_mode) {
    // 如果有保存的模式，直接使用
    kbSelectionMode.value = formData.value.config.kb_selection_mode;
  } else if (formData.value.config.knowledge_bases?.length > 0) {
    // 有指定知识库
    kbSelectionMode.value = 'selected';
  } else if (formData.value.config.allow_user_kb_selection) {
    // 允许用户选择（旧数据兼容：没有指定知识库但允许选择 = 全部）
    kbSelectionMode.value = 'all';
  } else {
    kbSelectionMode.value = 'none';
  }
};

// 监听知识库选择模式变化
watch(kbSelectionMode, (mode) => {
  formData.value.config.kb_selection_mode = mode;
  if (mode === 'none') {
    // 不使用知识库，清空相关配置
    formData.value.config.knowledge_bases = [];
    formData.value.config.allow_user_kb_selection = false;
  } else if (mode === 'all') {
    // 全部知识库，清空指定列表
    formData.value.config.knowledge_bases = [];
  }
  // selected 模式保持 knowledge_bases 不变
});

// 监听模式变化，自动调整配置
watch(agentMode, (val) => {
  if (val === 'agent') {
    // 切换到 Agent 模式，根据知识库配置启用工具
    if (formData.value.config.allowed_tools.length === 0) {
      if (hasKnowledgeBase.value) {
        // 有知识库时，启用所有工具
        formData.value.config.allowed_tools = [
          'thinking',
          'todo_write',
          'knowledge_search',
          'grep_chunks',
          'list_knowledge_chunks',
          'query_knowledge_graph',
          'get_document_info',
          'database_query',
        ];
      } else {
        // 没有知识库时，只启用非知识库工具
        formData.value.config.allowed_tools = ['thinking', 'todo_write'];
      }
    }
    if (formData.value.config.max_iterations <= 1) {
      formData.value.config.max_iterations = 10;
    }
  } else {
    // 切换到普通模式，清空工具
    formData.value.config.allowed_tools = [];
    formData.value.config.max_iterations = 1; // 设置为1表示单轮 RAG
  }
});

// 监听知识库配置变化，自动移除/添加知识库相关工具
watch(hasKnowledgeBase, (hasKB, oldHasKB) => {
  // 如果当前在检索策略页面但没有知识库能力了，切换到基础设置
  if (!hasKB && currentSection.value === 'retrieval') {
    currentSection.value = 'basic';
  }
  
  if (!isAgentMode.value) return; // 只在Agent模式下处理工具
  
  if (hasKB && !oldHasKB) {
    // 从无知识库变为有知识库，自动添加知识库相关工具
    const currentTools = formData.value.config.allowed_tools || [];
    const toolsToAdd = knowledgeBaseTools.filter((tool: string) => !currentTools.includes(tool));
    formData.value.config.allowed_tools = [...currentTools, ...toolsToAdd];
  } else if (!hasKB && oldHasKB) {
    // 从有知识库变为无知识库，移除知识库相关工具
    formData.value.config.allowed_tools = formData.value.config.allowed_tools.filter(
      (tool: string) => !knowledgeBaseTools.includes(tool)
    );
  }
});

// 监听运行模式变化，自动切换页面
watch(isAgentMode, (isAgent) => {
  // 如果当前在高级设置页面但切换到了Agent模式，切换到基础设置
  if (isAgent && currentSection.value === 'advanced') {
    currentSection.value = 'basic';
  }
  // 如果当前在多轮对话页面但切换到了Agent模式，切换到基础设置（Agent模式下多轮对话由内部控制）
  if (isAgent && currentSection.value === 'conversation') {
    currentSection.value = 'basic';
  }
});

// 加载依赖数据
const loadDependencies = async () => {
  try {
    // 加载模型列表 (只加载 KnowledgeQA 类型的模型)
    const models = await listModels('KnowledgeQA');
    if (models && models.length > 0) {
      modelOptions.value = models.map((m: any) => ({ label: m.name || m.id, value: m.id }));
    }

    // 加载 ReRank 模型列表
    const rerankModels = await listModels('Rerank');
    if (rerankModels && rerankModels.length > 0) {
      rerankModelOptions.value = rerankModels.map((m: any) => ({ label: m.name || m.id, value: m.id }));
    }

    // 加载知识库列表
    const kbRes: any = await listKnowledgeBases();
    if (kbRes.data) {
      kbOptions.value = kbRes.data.map((kb: any) => ({ label: kb.name, value: kb.id }));
    }

    // 加载可用占位符
    const agentConfig = await getAgentConfig();
    if (agentConfig.data?.available_placeholders) {
      availablePlaceholders.value = agentConfig.data.available_placeholders;
    }

    // 加载系统默认配置（用于内置智能体显示默认提示词）
    const conversationConfig = await getConversationConfig();
    if (conversationConfig.data?.prompt) {
      defaultSystemPrompt.value = conversationConfig.data.prompt;
    }
    if (conversationConfig.data?.context_template) {
      defaultContextTemplate.value = conversationConfig.data.context_template;
    }
    if (conversationConfig.data?.rewrite_prompt_system) {
      defaultRewritePromptSystem.value = conversationConfig.data.rewrite_prompt_system;
    }
    if (conversationConfig.data?.rewrite_prompt_user) {
      defaultRewritePromptUser.value = conversationConfig.data.rewrite_prompt_user;
    }
    if (conversationConfig.data?.fallback_prompt) {
      defaultFallbackPrompt.value = conversationConfig.data.fallback_prompt;
    }
    if (conversationConfig.data?.fallback_response) {
      defaultFallbackResponse.value = conversationConfig.data.fallback_response;
    }
    // 加载默认检索参数
    if (conversationConfig.data?.embedding_top_k) {
      defaultEmbeddingTopK.value = conversationConfig.data.embedding_top_k;
    }
    if (conversationConfig.data?.keyword_threshold !== undefined) {
      defaultKeywordThreshold.value = conversationConfig.data.keyword_threshold;
    }
    if (conversationConfig.data?.vector_threshold !== undefined) {
      defaultVectorThreshold.value = conversationConfig.data.vector_threshold;
    }
    if (conversationConfig.data?.rerank_top_k) {
      defaultRerankTopK.value = conversationConfig.data.rerank_top_k;
    }
    if (conversationConfig.data?.rerank_threshold !== undefined) {
      defaultRerankThreshold.value = conversationConfig.data.rerank_threshold;
    }
    if (conversationConfig.data?.max_completion_tokens) {
      defaultMaxCompletionTokens.value = conversationConfig.data.max_completion_tokens;
    }
    if (conversationConfig.data?.temperature !== undefined) {
      defaultTemperature.value = conversationConfig.data.temperature;
    }
  } catch (e) {
    console.error('Failed to load dependencies', e);
  }
};

const handleClose = () => {
  showPlaceholderPopup.value = false;
  showContextPlaceholderPopup.value = false;
  emit('update:visible', false);
};

// 过滤后的占位符列表
const filteredPlaceholders = computed(() => {
  if (!placeholderPrefix.value) {
    return availablePlaceholders.value;
  }
  const prefix = placeholderPrefix.value.toLowerCase();
  return availablePlaceholders.value.filter(p => 
    p.name.toLowerCase().startsWith(prefix)
  );
});

// 过滤后的上下文模板占位符列表
const filteredContextPlaceholders = computed(() => {
  if (!contextPlaceholderPrefix.value) {
    return contextTemplatePlaceholders.value;
  }
  const prefix = contextPlaceholderPrefix.value.toLowerCase();
  return contextTemplatePlaceholders.value.filter(p => 
    p.name.toLowerCase().startsWith(prefix)
  );
});

// 获取 textarea 元素
const getTextareaElement = (): HTMLTextAreaElement | null => {
  if (promptTextareaRef.value) {
    if (promptTextareaRef.value.$el) {
      return promptTextareaRef.value.$el.querySelector('textarea');
    }
    if (promptTextareaRef.value instanceof HTMLTextAreaElement) {
      return promptTextareaRef.value;
    }
  }
  return null;
};

// 计算光标位置
const calculateCursorPosition = (textarea: HTMLTextAreaElement) => {
  const cursorPos = textarea.selectionStart;
  const textBeforeCursor = formData.value.config.system_prompt.substring(0, cursorPos);
  
  const style = window.getComputedStyle(textarea);
  const textareaRect = textarea.getBoundingClientRect();
  
  const lineHeight = parseFloat(style.lineHeight) || 20;
  const paddingTop = parseFloat(style.paddingTop) || 0;
  const paddingLeft = parseFloat(style.paddingLeft) || 0;
  
  // 计算当前行号
  const lines = textBeforeCursor.split('\n');
  const currentLine = lines.length - 1;
  const currentLineText = lines[currentLine];
  
  // 创建临时 span 计算文本宽度
  const span = document.createElement('span');
  span.style.font = style.font;
  span.style.visibility = 'hidden';
  span.style.position = 'absolute';
  span.style.whiteSpace = 'pre';
  span.textContent = currentLineText;
  document.body.appendChild(span);
  const textWidth = span.offsetWidth;
  document.body.removeChild(span);
  
  const scrollTop = textarea.scrollTop;
  const top = textareaRect.top + paddingTop + (currentLine * lineHeight) - scrollTop + lineHeight + 4;
  const scrollLeft = textarea.scrollLeft;
  const left = textareaRect.left + paddingLeft + textWidth - scrollLeft;
  
  return { top, left };
};

// 检查并显示占位符提示
const checkAndShowPlaceholderPopup = () => {
  const textarea = getTextareaElement();
  if (!textarea) return;
  
  const cursorPos = textarea.selectionStart;
  const textBeforeCursor = formData.value.config.system_prompt.substring(0, cursorPos);
  
  // 查找最近的 {{ 位置
  let lastOpenPos = -1;
  for (let i = textBeforeCursor.length - 1; i >= 1; i--) {
    if (textBeforeCursor[i] === '{' && textBeforeCursor[i - 1] === '{') {
      const textAfterOpen = textBeforeCursor.substring(i + 1);
      if (!textAfterOpen.includes('}}')) {
        lastOpenPos = i - 1;
        break;
      }
    }
  }
  
  if (lastOpenPos === -1) {
    showPlaceholderPopup.value = false;
    placeholderPrefix.value = '';
    return;
  }
  
  const textAfterOpen = textBeforeCursor.substring(lastOpenPos + 2);
  placeholderPrefix.value = textAfterOpen;
  
  const filtered = filteredPlaceholders.value;
  if (filtered.length > 0) {
    nextTick(() => {
      const position = calculateCursorPosition(textarea);
      popupStyle.value = {
        top: `${position.top}px`,
        left: `${position.left}px`
      };
      showPlaceholderPopup.value = true;
      selectedPlaceholderIndex.value = 0;
    });
  } else {
    showPlaceholderPopup.value = false;
  }
};

// 处理输入
const handlePromptInput = () => {
  if (placeholderPopupTimer) {
    clearTimeout(placeholderPopupTimer);
  }
  placeholderPopupTimer = setTimeout(() => {
    checkAndShowPlaceholderPopup();
  }, 50);
};

// 插入占位符
const insertPlaceholder = (placeholderName: string) => {
  const textarea = getTextareaElement();
  if (!textarea) return;
  
  showPlaceholderPopup.value = false;
  placeholderPrefix.value = '';
  selectedPlaceholderIndex.value = 0;
  
  nextTick(() => {
    const cursorPos = textarea.selectionStart;
    const currentValue = formData.value.config.system_prompt;
    const textBeforeCursor = currentValue.substring(0, cursorPos);
    const textAfterCursor = currentValue.substring(cursorPos);
    
    // 找到 {{ 的位置
    let lastOpenPos = -1;
    for (let i = textBeforeCursor.length - 1; i >= 1; i--) {
      if (textBeforeCursor[i] === '{' && textBeforeCursor[i - 1] === '{') {
        lastOpenPos = i - 1;
        break;
      }
    }
    
    if (lastOpenPos !== -1) {
      const textBeforeOpen = currentValue.substring(0, lastOpenPos);
      const newValue = textBeforeOpen + `{{${placeholderName}}}` + textAfterCursor;
      formData.value.config.system_prompt = newValue;
      
      nextTick(() => {
        const newCursorPos = textBeforeOpen.length + placeholderName.length + 4;
        textarea.setSelectionRange(newCursorPos, newCursorPos);
        textarea.focus();
      });
    }
  });
};

// 获取上下文模板 textarea 元素
const getContextTemplateTextareaElement = (): HTMLTextAreaElement | null => {
  if (contextTemplateTextareaRef.value) {
    if (contextTemplateTextareaRef.value.$el) {
      return contextTemplateTextareaRef.value.$el.querySelector('textarea');
    }
    if (contextTemplateTextareaRef.value instanceof HTMLTextAreaElement) {
      return contextTemplateTextareaRef.value;
    }
  }
  return null;
};

// 计算上下文模板光标位置
const calculateContextCursorPosition = (textarea: HTMLTextAreaElement) => {
  const cursorPos = textarea.selectionStart;
  const textBeforeCursor = formData.value.config.context_template.substring(0, cursorPos);
  
  const style = window.getComputedStyle(textarea);
  const textareaRect = textarea.getBoundingClientRect();
  
  const lineHeight = parseFloat(style.lineHeight) || 20;
  const paddingTop = parseFloat(style.paddingTop) || 0;
  const paddingLeft = parseFloat(style.paddingLeft) || 0;
  
  const lines = textBeforeCursor.split('\n');
  const currentLine = lines.length - 1;
  const currentLineText = lines[currentLine];
  
  const span = document.createElement('span');
  span.style.font = style.font;
  span.style.visibility = 'hidden';
  span.style.position = 'absolute';
  span.style.whiteSpace = 'pre';
  span.textContent = currentLineText;
  document.body.appendChild(span);
  const textWidth = span.offsetWidth;
  document.body.removeChild(span);
  
  const scrollTop = textarea.scrollTop;
  const top = textareaRect.top + paddingTop + (currentLine * lineHeight) - scrollTop + lineHeight + 4;
  const scrollLeft = textarea.scrollLeft;
  const left = textareaRect.left + paddingLeft + textWidth - scrollLeft;
  
  return { top, left };
};

// 检查并显示上下文模板占位符提示
const checkAndShowContextPlaceholderPopup = () => {
  const textarea = getContextTemplateTextareaElement();
  if (!textarea) return;
  
  const cursorPos = textarea.selectionStart;
  const textBeforeCursor = formData.value.config.context_template.substring(0, cursorPos);
  
  let lastOpenPos = -1;
  for (let i = textBeforeCursor.length - 1; i >= 1; i--) {
    if (textBeforeCursor[i] === '{' && textBeforeCursor[i - 1] === '{') {
      const textAfterOpen = textBeforeCursor.substring(i + 1);
      if (!textAfterOpen.includes('}}')) {
        lastOpenPos = i - 1;
        break;
      }
    }
  }
  
  if (lastOpenPos === -1) {
    showContextPlaceholderPopup.value = false;
    contextPlaceholderPrefix.value = '';
    return;
  }
  
  const textAfterOpen = textBeforeCursor.substring(lastOpenPos + 2);
  contextPlaceholderPrefix.value = textAfterOpen;
  
  const filtered = filteredContextPlaceholders.value;
  if (filtered.length > 0) {
    nextTick(() => {
      const position = calculateContextCursorPosition(textarea);
      contextPopupStyle.value = {
        top: `${position.top}px`,
        left: `${position.left}px`
      };
      showContextPlaceholderPopup.value = true;
      selectedContextPlaceholderIndex.value = 0;
    });
  } else {
    showContextPlaceholderPopup.value = false;
  }
};

// 处理上下文模板输入
const handleContextTemplateInput = () => {
  if (contextPlaceholderPopupTimer) {
    clearTimeout(contextPlaceholderPopupTimer);
  }
  contextPlaceholderPopupTimer = setTimeout(() => {
    checkAndShowContextPlaceholderPopup();
  }, 50);
};

// 插入上下文模板占位符
const insertContextPlaceholder = (placeholderName: string) => {
  const textarea = getContextTemplateTextareaElement();
  if (!textarea) return;
  
  showContextPlaceholderPopup.value = false;
  contextPlaceholderPrefix.value = '';
  selectedContextPlaceholderIndex.value = 0;
  
  nextTick(() => {
    const cursorPos = textarea.selectionStart;
    const currentValue = formData.value.config.context_template;
    const textBeforeCursor = currentValue.substring(0, cursorPos);
    const textAfterCursor = currentValue.substring(cursorPos);
    
    let lastOpenPos = -1;
    for (let i = textBeforeCursor.length - 1; i >= 1; i--) {
      if (textBeforeCursor[i] === '{' && textBeforeCursor[i - 1] === '{') {
        lastOpenPos = i - 1;
        break;
      }
    }
    
    if (lastOpenPos !== -1) {
      const textBeforeOpen = currentValue.substring(0, lastOpenPos);
      const newValue = textBeforeOpen + `{{${placeholderName}}}` + textAfterCursor;
      formData.value.config.context_template = newValue;
      
      nextTick(() => {
        const newCursorPos = textBeforeOpen.length + placeholderName.length + 4;
        textarea.setSelectionRange(newCursorPos, newCursorPos);
        textarea.focus();
      });
    }
  });
};

// 设置上下文模板 textarea 事件监听
const setupContextTemplateEventListeners = () => {
  nextTick(() => {
    const textarea = getContextTemplateTextareaElement();
    if (textarea) {
      textarea.addEventListener('keydown', (e: KeyboardEvent) => {
        if (showContextPlaceholderPopup.value && filteredContextPlaceholders.value.length > 0) {
          if (e.key === 'ArrowDown') {
            e.preventDefault();
            e.stopPropagation();
            if (selectedContextPlaceholderIndex.value < filteredContextPlaceholders.value.length - 1) {
              selectedContextPlaceholderIndex.value++;
            } else {
              selectedContextPlaceholderIndex.value = 0;
            }
          } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            e.stopPropagation();
            if (selectedContextPlaceholderIndex.value > 0) {
              selectedContextPlaceholderIndex.value--;
            } else {
              selectedContextPlaceholderIndex.value = filteredContextPlaceholders.value.length - 1;
            }
          } else if (e.key === 'Enter' || e.key === 'Tab') {
            e.preventDefault();
            e.stopPropagation();
            const selected = filteredContextPlaceholders.value[selectedContextPlaceholderIndex.value];
            if (selected) {
              insertContextPlaceholder(selected.name);
            }
          } else if (e.key === 'Escape') {
            e.preventDefault();
            e.stopPropagation();
            showContextPlaceholderPopup.value = false;
            contextPlaceholderPrefix.value = '';
          }
        }
      }, true);
    }
  });
};

// 设置 textarea 事件监听
const setupTextareaEventListeners = () => {
  nextTick(() => {
    const textarea = getTextareaElement();
    if (textarea) {
      textarea.addEventListener('keydown', (e: KeyboardEvent) => {
        if (showPlaceholderPopup.value && filteredPlaceholders.value.length > 0) {
          if (e.key === 'ArrowDown') {
            e.preventDefault();
            e.stopPropagation();
            if (selectedPlaceholderIndex.value < filteredPlaceholders.value.length - 1) {
              selectedPlaceholderIndex.value++;
            } else {
              selectedPlaceholderIndex.value = 0;
            }
          } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            e.stopPropagation();
            if (selectedPlaceholderIndex.value > 0) {
              selectedPlaceholderIndex.value--;
            } else {
              selectedPlaceholderIndex.value = filteredPlaceholders.value.length - 1;
            }
          } else if (e.key === 'Enter' || e.key === 'Tab') {
            e.preventDefault();
            e.stopPropagation();
            const selected = filteredPlaceholders.value[selectedPlaceholderIndex.value];
            if (selected) {
              insertPlaceholder(selected.name);
            }
          } else if (e.key === 'Escape') {
            e.preventDefault();
            e.stopPropagation();
            showPlaceholderPopup.value = false;
            placeholderPrefix.value = '';
          }
        }
      }, true);
    }
  });
};

// 监听 visible 变化设置事件监听
watch(() => props.visible, (val) => {
  if (val) {
    nextTick(() => {
      setupTextareaEventListeners();
      setupContextTemplateEventListeners();
    });
  }
});

// 模板选择处理函数
const handleSystemPromptTemplateSelect = (template: string) => {
  if (activePromptTab.value === 'web-enabled') {
    formData.value.config.system_prompt_web_enabled = template;
  } else {
    formData.value.config.system_prompt = template;
  }
};

const handleContextTemplateSelect = (template: string) => {
  formData.value.config.context_template = template;
};

const handleRewriteSystemTemplateSelect = (template: string) => {
  formData.value.config.rewrite_prompt_system = template;
};

const handleRewriteUserTemplateSelect = (template: string) => {
  formData.value.config.rewrite_prompt_user = template;
};

const handleFallbackResponseTemplateSelect = (template: string) => {
  formData.value.config.fallback_response = template;
};

const handleFallbackPromptTemplateSelect = (template: string) => {
  formData.value.config.fallback_prompt = template;
};

const handleSave = async () => {
  // 验证必填项（内置智能体不验证名称和系统提示词）
  if (!isBuiltinAgent.value) {
    if (!formData.value.name || !formData.value.name.trim()) {
      MessagePlugin.error(t('agent.editor.nameRequired'));
      currentSection.value = 'basic';
      return;
    }

    // 自定义智能体必须填写系统提示词
    if (!formData.value.config.system_prompt || !formData.value.config.system_prompt.trim()) {
      MessagePlugin.error(t('agent.editor.systemPromptRequired'));
      currentSection.value = 'basic';
      return;
    }

    // 自定义智能体普通模式必须填写上下文模板
    if (!isAgentMode.value && (!formData.value.config.context_template || !formData.value.config.context_template.trim())) {
      MessagePlugin.error(t('agent.editor.contextTemplateRequired'));
      currentSection.value = 'basic';
      return;
    }
  }

  if (!formData.value.config.model_id) {
    MessagePlugin.error(t('agent.editor.modelRequired'));
    currentSection.value = 'model';
    return;
  }

  // 校验 ReRank 模型（当需要时必填）
  if (needsRerankModel.value && !formData.value.config.rerank_model_id) {
    MessagePlugin.error(t('agent.editor.rerankModelRequired'));
    currentSection.value = 'knowledge';
    return;
  }

  // 过滤空推荐问题
  if (formData.value.config.suggested_prompts) {
    formData.value.config.suggested_prompts = formData.value.config.suggested_prompts.filter((p: string) => p.trim() !== '');
  }

  // 确保类型设置正确（内置智能体保持原类型）
  if (!isBuiltinAgent.value) {
    formData.value.type = 'custom';
  }

  saving.value = true;
  try {
    if (props.mode === 'create') {
      await createAgent(formData.value);
      MessagePlugin.success(t('agent.messages.created'));
    } else {
      await updateAgent(formData.value.id, formData.value);
      MessagePlugin.success(t('agent.messages.updated'));
    }
    emit('success');
    handleClose();
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('agent.messages.saveFailed'));
  } finally {
    saving.value = false;
  }
};
</script>

<style scoped lang="less">
// 复用创建知识库的样式
.settings-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
}

.settings-modal {
  position: relative;
  width: 90vw;
  max-width: 1100px;
  height: 85vh;
  max-height: 750px;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.12);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.close-btn {
  position: absolute;
  top: 20px;
  right: 20px;
  width: 32px;
  height: 32px;
  border: none;
  background: #f5f5f5;
  border-radius: 6px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #666;
  transition: all 0.2s ease;
  z-index: 10;

  &:hover {
    background: #e5e5e5;
    color: #000;
  }
}

.settings-container {
  display: flex;
  height: 100%;
  overflow: hidden;
}

.settings-sidebar {
  width: 200px;
  background: #fafafa;
  border-right: 1px solid #e5e5e5;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.sidebar-header {
  padding: 24px 20px;
  border-bottom: 1px solid #e5e5e5;
}

.sidebar-title {
  margin: 0;
  font-family: "PingFang SC";
  font-size: 18px;
  font-weight: 600;
  color: #000000e6;
}

.settings-nav {
  flex: 1;
  padding: 12px 8px;
  overflow-y: auto;
}

.nav-item {
  display: flex;
  align-items: center;
  padding: 10px 12px;
  margin-bottom: 4px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
  font-family: "PingFang SC";
  font-size: 14px;
  color: #00000099;

  &:hover {
    background: #f0f0f0;
  }

  &.active {
    background: #07c05f1a;
    color: #07c05f;
    font-weight: 500;
  }
}

.nav-icon {
  margin-right: 8px;
  font-size: 18px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.nav-label {
  flex: 1;
}

.settings-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.content-wrapper {
  flex: 1;
  overflow-y: auto;
  padding: 24px 32px;
}

.section {
  width: 100%;
}

// 与知识库设置一致的 section-header 样式
.section-header {
  margin-bottom: 32px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: #333333;
    margin: 0 0 8px 0;
  }

  .section-description {
    font-size: 14px;
    color: #666666;
    margin: 0;
    line-height: 1.5;
  }
}

// 与知识库设置一致的 settings-group 样式
.settings-group {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 20px 0;
  border-bottom: 1px solid #e5e7eb;

  &:last-child {
    border-bottom: none;
  }

  &.setting-row-vertical {
    flex-direction: column;
    gap: 12px;
  }
}

.setting-info {
  flex: 1;
  max-width: 55%;
  padding-right: 24px;

  &.full-width {
    max-width: 100%;
    padding-right: 0;
  }

  .setting-info-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 4px;
    
    label {
      margin-bottom: 0;
    }
  }

  label {
    font-size: 15px;
    font-weight: 500;
    color: #333333;
    display: block;
    margin-bottom: 4px;

    .required {
      color: #fa5151;
      margin-left: 2px;
    }
  }

  .desc {
    font-size: 13px;
    color: #666666;
    margin: 0;
    line-height: 1.5;
  }
}

.setting-control {
  flex-shrink: 0;
  min-width: 360px;
  display: flex;
  justify-content: flex-end;
  align-items: flex-start;

  &.setting-control-full {
    width: 100%;
    min-width: 100%;
    justify-content: flex-start;
  }

  // 让 select 和 input 占满控件区域
  :deep(.t-select),
  :deep(.t-input),
  :deep(.t-textarea) {
    width: 100%;
  }

  :deep(.t-input-number) {
    width: 120px;
  }
}

// 名称输入框带头像预览
.name-input-wrapper {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;

  .name-input {
    flex: 1;
  }
}

.settings-footer {
  padding: 16px 32px;
  border-top: 1px solid #e5e5e5;
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  flex-shrink: 0;
}

// 模式提示样式
.mode-hint {
  display: flex;
  align-items: center;
  padding: 10px 14px;
  background: #f0faf5;
  border-radius: 6px;
  border: 1px solid #d4f0e2;
  color: #07c05f;
  font-size: 13px;
  line-height: 1.5;
}

// 过渡动画
.modal-enter-active,
.modal-leave-active {
  transition: all 0.3s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;

  .settings-modal {
    transform: scale(0.95);
  }
}

// Slider 样式
.slider-wrapper {
  display: flex;
  align-items: center;
  gap: 16px;
  width: 100%;

  :deep(.t-slider) {
    flex: 1;
  }
}

.slider-value {
  width: 40px;
  text-align: right;
  font-family: monospace;
  font-size: 14px;
  color: #333;
}

// 推荐问题列表
.suggested-prompts-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.prompt-item {
  display: flex;
  align-items: center;
  gap: 8px;

  :deep(.t-input) {
    flex: 1;
  }
}

// Radio-group 样式优化，符合项目主题风格
:deep(.t-radio-group) {
  .t-radio-group--filled {
    background: #f5f5f5;
  }
  .t-radio-button {
    border-color: #d9d9d9;

    &:hover:not(.t-is-disabled) {
      border-color: #07c05f;
      color: #07c05f;
    }

    &.t-is-checked {
      background: #07c05f;
      border-color: #07c05f;
      color: #fff;

      &:hover:not(.t-is-disabled) {
        background: #05a04f;
        border-color: #05a04f;
        color: #fff;
      }
    }

    // 禁用状态样式
    &.t-is-disabled {
      background: #f5f5f5;
      border-color: #d9d9d9;
      color: #00000040;
      cursor: not-allowed;
      opacity: 0.6;

      &.t-is-checked {
        background: #f0f0f0;
        border-color: #d9d9d9;
        color: #00000066;
      }
    }
  }
}

// 工具选择样式
.tools-checkbox-group {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  width: 100%;
}

.tool-checkbox-item {
  display: flex;
  align-items: flex-start;
  padding: 12px 16px;
  background: #fafafa;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  transition: all 0.2s ease;

  &:hover {
    border-color: #07c05f;
    background: #f0faf5;
  }

  :deep(.t-checkbox__input) {
    margin-top: 2px;
  }

  :deep(.t-checkbox__label) {
    flex: 1;
  }
}

.tool-item-content {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.tool-name {
  font-size: 14px;
  font-weight: 500;
  color: #333;
}

.tool-desc {
  font-size: 12px;
  color: #666;
  line-height: 1.5;
}

.tool-disabled-hint {
  font-size: 11px;
  color: #f5a623;
  font-style: italic;
}

.tool-disabled {
  opacity: 0.6;
  
  .tool-name, .tool-desc {
    color: #999;
  }
}

// Checkbox 选中样式
:deep(.t-checkbox) {
  &.t-is-checked {
    .t-checkbox__input {
      border-color: #07c05f;
      background-color: #07c05f;
    }
  }
  
  &:hover:not(.t-is-disabled) {
    .t-checkbox__input {
      border-color: #07c05f;
    }
  }
}

// Switch 样式
:deep(.t-switch) {
  &.t-is-checked {
    background-color: #07c05f;
    
    &:hover:not(.t-is-disabled) {
      background-color: #05a04f;
    }
  }
}

// Slider 样式
:deep(.t-slider) {
  .t-slider__track {
    background-color: #07c05f;
  }
  
  .t-slider__button {
    border-color: #07c05f;
  }
}

// Button 主题样式
:deep(.t-button--theme-primary) {
  background-color: #07c05f;
  border-color: #07c05f;
  
  &:hover:not(.t-is-disabled) {
    background-color: #05a04f;
    border-color: #05a04f;
  }
}

// Input/Select focus 样式
:deep(.t-input),
:deep(.t-textarea),
:deep(.t-select) {
  &.t-is-focused,
  &:focus-within {
    border-color: #07c05f;
    box-shadow: 0 0 0 2px rgba(7, 192, 95, 0.1);
  }
}

// textarea 与模板选择器容器
.textarea-with-template {
  position: relative;
  width: 100%;
}

// 系统提示词输入框样式
.system-prompt-textarea {
  width: 100%;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 13px;

  :deep(textarea) {
    resize: vertical !important;
    min-height: 200px;
  }
}

// 占位符提示样式
.placeholder-hint {
  margin-top: 12px;
  padding: 12px;
  background: #f8fafc;
  border-radius: 6px;
  border: 1px solid #e1e8ed;
  font-size: 12px;
  max-width: 480px;

  .hint-title {
    font-weight: 500;
    color: #333;
    margin: 0 0 8px 0;
  }

  .placeholder-list {
    margin: 8px 0;
    padding-left: 20px;
    color: #666;

    li {
      margin: 4px 0;

      code {
        background: #fff;
        padding: 2px 6px;
        border-radius: 3px;
        font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
        font-size: 11px;
        color: #e83e8c;
        border: 1px solid #e1e8ed;
      }
    }
  }

  .hint-tip {
    margin: 8px 0 0 0;
    color: #999;
    font-style: italic;
  }
}

.placeholder-popup-wrapper {
  position: fixed;
  z-index: 10001;
  pointer-events: auto;
}

.placeholder-popup {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  max-width: 400px;
  max-height: 300px;
  overflow-y: auto;
  padding: 4px 0;
}

.placeholder-item {
  padding: 8px 12px;
  cursor: pointer;
  transition: background-color 0.2s;

  &:hover,
  &.active {
    background-color: #f5f7fa;
  }

  .placeholder-name {
    font-weight: 500;
    margin-bottom: 4px;

    code {
      background: #f5f7fa;
      padding: 2px 6px;
      border-radius: 3px;
      font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
      font-size: 12px;
      color: #e83e8c;
    }
  }

  .placeholder-desc {
    font-size: 12px;
    color: #666;
  }
}

// 内置智能体提示
.builtin-agent-notice {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: #fff7e6;
  border: 1px solid #ffd591;
  border-radius: 8px;
  margin-bottom: 16px;
  color: #d46b08;
  font-size: 14px;

  .t-icon {
    font-size: 16px;
    flex-shrink: 0;
  }
}

// 提示词开关
.prompt-toggle {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 12px;

  .prompt-toggle-label {
    font-size: 13px;
    color: #666;
  }
}

// 提示词禁用提示
.prompt-disabled-hint {
  color: #999;
  font-size: 13px;
  font-style: italic;
  padding: 12px 16px;
  background: #f5f5f5;
  border-radius: 6px;
}

// 系统提示词Tabs
.system-prompt-tabs {
  width: 100%;

  .prompt-variant-tabs {
    :deep(.t-tabs__nav) {
      margin-bottom: 12px;
    }
  }
}
</style>
