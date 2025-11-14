<template>
  <div class="faq-manager">
    <!-- Header -->
    <div class="faq-header">
      <div class="faq-header-title">
        <h2>{{ $t('knowledgeEditor.faq.title') }}</h2>
        <p class="faq-subtitle">{{ $t('knowledgeEditor.faq.subtitle') }}</p>
      </div>
      <div class="action-buttons">
        <button class="create-btn ghost" @click="openEditor()">
          <t-icon name="add" size="16px" class="btn-icon" />
          <span>{{ $t('knowledgeEditor.faq.editorCreate') }}</span>
        </button>
        <t-dropdown
          :options="toolbarActionOptions"
          placement="bottom-left"
          trigger="click"
          @click="handleToolbarAction"
        >
          <div class="toolbar-action-trigger">
            <t-icon name="more" />
          </div>
        </t-dropdown>
      </div>
    </div>

    <!-- Card List Container with Scroll -->
    <div ref="scrollContainer" class="faq-scroll-container" @scroll="handleScroll">
      <t-loading :loading="loading && entries.length === 0" size="medium">
        <!-- Card List -->
        <div v-if="entries.length > 0" ref="cardListRef" class="faq-card-list">
        <div
          v-for="entry in entries"
          :key="entry.id"
          class="faq-card"
          :class="{ 'selected': selectedRowKeys.includes(entry.id) }"
          @click="handleCardSelect(entry.id, !selectedRowKeys.includes(entry.id))"
        >
          <!-- Card Header -->
          <div class="faq-card-header">
            <div class="faq-question" :title="entry.standard_question">
              {{ entry.standard_question }}
            </div>
            <t-popup
              v-model="entry.showMore"
              overlayClassName="faq-card-popup"
              trigger="click"
              destroy-on-close
              placement="bottom-right"
              @visible-change="(visible: boolean) => entry.showMore = visible"
            >
              <div class="card-more-btn" @click.stop>
                <img class="more-icon" src="@/assets/img/more.png" alt="" />
              </div>
              <template #content>
                <div class="card-menu" @click.stop>
                  <div class="card-menu-item" @click.stop="handleMenuEdit(entry)">
                    <t-icon class="menu-icon" name="edit" />
                    <span>{{ $t('common.edit') }}</span>
                  </div>
                  <div class="card-menu-item danger" @click.stop="handleMenuDelete(entry)">
                    <t-icon class="menu-icon" name="delete" />
                    <span>{{ $t('common.delete') }}</span>
                  </div>
                </div>
              </template>
            </t-popup>
          </div>

          <!-- Card Body -->
          <div class="faq-card-body">
            <!-- Similar Questions Section -->
            <div v-if="entry.similar_questions?.length" class="faq-section similar">
              <div 
                class="faq-section-label clickable"
                @click.stop="entry.similarCollapsed = !entry.similarCollapsed"
              >
                <span>{{ $t('knowledgeEditor.faq.similarQuestions') }}</span>
                <span class="section-count">
                  ({{ entry.similar_questions.length }})
                </span>
                <t-icon 
                  :name="entry.similarCollapsed ? 'chevron-right' : 'chevron-down'" 
                  class="collapse-icon"
                />
              </div>
              <Transition name="slide-down">
                <div v-if="!entry.similarCollapsed" class="faq-tags">
                  <FAQTagTooltip
                    v-for="question in entry.similar_questions"
                    :key="question"
                    :content="question"
                    type="similar"
                    placement="top"
                  >
                    <t-tag
                      size="small"
                      variant="light-outline"
                      class="question-tag"
                    >
                      {{ question }}
                    </t-tag>
                  </FAQTagTooltip>
                </div>
              </Transition>
            </div>

            <!-- Negative Questions Section -->
            <div v-if="entry.negative_questions?.length" class="faq-section negative">
              <div 
                class="faq-section-label clickable"
                @click.stop="entry.negativeCollapsed = !entry.negativeCollapsed"
              >
                <span>{{ $t('knowledgeEditor.faq.negativeQuestions') }}</span>
                <span class="section-count">
                  ({{ entry.negative_questions.length }})
                </span>
                <t-icon 
                  :name="entry.negativeCollapsed ? 'chevron-right' : 'chevron-down'" 
                  class="collapse-icon"
                />
              </div>
              <Transition name="slide-down">
                <div v-if="!entry.negativeCollapsed" class="faq-tags">
                  <FAQTagTooltip
                    v-for="question in entry.negative_questions"
                    :key="question"
                    :content="question"
                    type="negative"
                    placement="top"
                  >
                    <t-tag
                      size="small"
                      theme="warning"
                      variant="light-outline"
                      class="question-tag"
                    >
                      {{ question }}
                    </t-tag>
                  </FAQTagTooltip>
                </div>
              </Transition>
            </div>

            <!-- Answers Section -->
            <div class="faq-section answers">
              <div 
                class="faq-section-label clickable"
                @click.stop="entry.answersCollapsed = !entry.answersCollapsed"
              >
                <span>{{ $t('knowledgeEditor.faq.answers') }}</span>
                <span v-if="entry.answers?.length" class="section-count">
                  ({{ entry.answers.length }})
                </span>
                <t-icon 
                  :name="entry.answersCollapsed ? 'chevron-right' : 'chevron-down'" 
                  class="collapse-icon"
                />
              </div>
              <Transition name="slide-down">
                <div v-if="!entry.answersCollapsed" class="faq-tags">
                  <FAQTagTooltip
                    v-for="answer in entry.answers"
                    :key="answer"
                    :content="answer"
                    type="answer"
                    placement="top"
                  >
                    <t-tag
                      size="small"
                      variant="light-outline"
                      class="question-tag"
                    >
                      {{ answer }}
                    </t-tag>
                  </FAQTagTooltip>
                  <span v-if="!entry.answers?.length" class="empty-tip">
                    {{ $t('knowledgeEditor.faq.noAnswer') }}
                  </span>
                </div>
              </Transition>
            </div>
          </div>

        </div>
      </div>

        <!-- Empty State -->
        <div v-else-if="!loading" class="faq-empty-state">
          <div class="empty-content">
            <t-icon name="file-add" size="48px" class="empty-icon" />
            <div class="empty-text">{{ $t('knowledgeEditor.faq.emptyTitle') }}</div>
            <div class="empty-desc">{{ $t('knowledgeEditor.faq.emptyDesc') }}</div>
          </div>
        </div>
      </t-loading>

      <!-- Load More Indicator -->
      <div v-if="loadingMore" class="faq-load-more">
        <t-loading size="small" :text="$t('common.loading')" />
      </div>
      <div v-if="hasMore === false && entries.length > 0" class="faq-no-more">
        {{ $t('common.noMoreData') }}
      </div>
    </div>

    <!-- Editor Drawer -->
    <t-drawer
      v-model:visible="editorVisible"
      :header="editorMode === 'create' ? $t('knowledgeEditor.faq.editorCreate') : $t('knowledgeEditor.faq.editorEdit')"
      :close-btn="true"
      size="520px"
      placement="right"
      class="faq-editor-drawer"
      @close="handleEditorClose"
    >
      <div class="faq-editor-drawer-content">
        <t-form
          ref="editorFormRef"
          :data="editorForm"
          :rules="editorRules"
          layout="vertical"
          class="editor-form"
        >
          <t-form-item name="standard_question" :label="$t('knowledgeEditor.faq.standardQuestion')">
            <t-input v-model="editorForm.standard_question" :maxlength="200" />
          </t-form-item>

          <t-form-item name="similar_questions" :label="$t('knowledgeEditor.faq.similarQuestions')">
            <div class="full-width-input-wrapper">
              <t-input
                v-model="similarInput"
                :placeholder="$t('knowledgeEditor.faq.similarPlaceholder')"
                @keydown.enter.prevent="addSimilar"
                class="full-width-input"
              />
              <t-button
                theme="primary"
                variant="outline"
                :disabled="!similarInput.trim() || editorForm.similar_questions.length >= 10"
                @click="addSimilar"
                class="add-item-btn"
                size="small"
              >
                <t-icon name="add" size="16px" />
              </t-button>
            </div>
            <div v-if="editorForm.similar_questions.length > 0" class="item-list">
              <div
                v-for="(question, index) in editorForm.similar_questions"
                :key="index"
                class="item-row"
              >
                <div class="item-content">{{ question }}</div>
                <t-button
                  theme="default"
                  variant="text"
                  size="small"
                  @click="removeSimilar(index)"
                  class="remove-item-btn"
                >
                  <t-icon name="close" size="16px" />
                </t-button>
              </div>
            </div>
          </t-form-item>

          <div class="section-divider"></div>

          <t-form-item name="negative_questions" :label="$t('knowledgeEditor.faq.negativeQuestions')">
            <div class="full-width-input-wrapper">
              <t-input
                v-model="negativeInput"
                :placeholder="$t('knowledgeEditor.faq.negativePlaceholder')"
                @keydown.enter.prevent="addNegative"
                class="full-width-input"
              />
              <t-button
                theme="primary"
                variant="outline"
                :disabled="!negativeInput.trim() || editorForm.negative_questions.length >= 10"
                @click="addNegative"
                class="add-item-btn"
                size="small"
              >
                <t-icon name="add" size="16px" />
              </t-button>
            </div>
            <div v-if="editorForm.negative_questions.length > 0" class="item-list">
              <div
                v-for="(question, index) in editorForm.negative_questions"
                :key="index"
                class="item-row negative"
              >
                <div class="item-content">{{ question }}</div>
                <t-button
                  theme="default"
                  variant="text"
                  size="small"
                  @click="removeNegative(index)"
                  class="remove-item-btn"
                >
                  <t-icon name="close" size="16px" />
                </t-button>
              </div>
            </div>
          </t-form-item>

          <div class="section-divider"></div>

          <t-form-item name="answers" :label="$t('knowledgeEditor.faq.answers')">
            <div class="full-width-input-wrapper textarea-wrapper">
              <t-textarea
                v-model="answerInput"
                :placeholder="$t('knowledgeEditor.faq.answerPlaceholder')"
                :autosize="{ minRows: 3, maxRows: 6 }"
                class="full-width-textarea"
                @keydown.ctrl.enter="addAnswer"
                @keydown.meta.enter="addAnswer"
              />
              <t-button
                theme="primary"
                variant="outline"
                :disabled="!answerInput.trim() || editorForm.answers.length >= 5"
                @click="addAnswer"
                class="add-item-btn"
                size="small"
              >
                <t-icon name="add" size="16px" />
              </t-button>
            </div>
            <div class="item-count">{{ editorForm.answers.length }}/5</div>
            <div v-if="editorForm.answers.length > 0" class="item-list">
              <div
                v-for="(answer, index) in editorForm.answers"
                :key="index"
                class="item-row answer-row"
              >
                <div class="item-content">{{ answer }}</div>
                <t-button
                  theme="default"
                  variant="text"
                  size="small"
                  @click="removeAnswer(index)"
                  class="remove-item-btn"
                >
                  <t-icon name="close" size="16px" />
                </t-button>
              </div>
            </div>
          </t-form-item>
        </t-form>
      </div>

      <template #footer>
        <div class="faq-editor-drawer-footer">
          <t-button theme="default" variant="outline" @click="editorVisible = false">
            {{ $t('common.cancel') }}
          </t-button>
          <t-button theme="primary" @click="handleSubmitEntry" :loading="savingEntry">
            {{ editorMode === 'create' ? $t('knowledgeEditor.faq.editorCreate') : $t('common.save') }}
          </t-button>
        </div>
      </template>
    </t-drawer>

    <!-- Import Dialog -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="importVisible" class="faq-import-overlay" @click.self="importVisible = false">
          <div class="faq-import-modal">
            <!-- 关闭按钮 -->
            <button class="close-btn" @click="importVisible = false" :aria-label="$t('general.close')">
              <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
                <path d="M15 5L5 15M5 5L15 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
              </svg>
            </button>

            <div class="faq-import-container">
              <div class="faq-import-header">
                <h2 class="import-title">{{ $t('knowledgeEditor.faqImport.title') }}</h2>
              </div>

              <div class="faq-import-content">
                <!-- 导入模式选择 -->
                <div class="import-form-item">
                  <label class="import-form-label required">{{ $t('knowledgeEditor.faqImport.modeLabel') }}</label>
                  <t-radio-group v-model="importState.mode" variant="default-filled" class="import-radio-group">
                    <t-radio-button value="append">{{ $t('knowledgeEditor.faqImport.appendMode') }}</t-radio-button>
                    <t-radio-button value="replace">{{ $t('knowledgeEditor.faqImport.replaceMode') }}</t-radio-button>
                  </t-radio-group>
                </div>

                <!-- 文件上传区域 -->
                <div class="import-form-item">
                  <label class="import-form-label required">{{ $t('knowledgeEditor.faqImport.fileLabel') }}</label>
                  <div class="file-upload-wrapper">
                    <input
                      ref="fileInputRef"
                      type="file"
                      accept=".json,.csv,.xlsx,.xls"
                      @change="handleFileChange"
                      class="file-input-hidden"
                    />
                    <div
                      class="file-upload-area"
                      :class="{ 'has-file': importState.file }"
                      @click="fileInputRef?.click()"
                      @dragover.prevent
                      @dragenter.prevent
                      @drop.prevent="handleFileDrop"
                    >
                      <div class="file-upload-content">
                        <t-icon name="upload" size="32px" class="upload-icon" />
                        <div class="upload-text">
                          <span v-if="!importState.file" class="upload-primary-text">
                            {{ $t('knowledgeEditor.faqImport.clickToUpload') }}
                          </span>
                          <span v-else class="upload-file-name">
                            {{ importState.file.name }}
                          </span>
                          <span v-if="!importState.file" class="upload-secondary-text">
                            {{ $t('knowledgeEditor.faqImport.dragDropTip') }}
                          </span>
                        </div>
                      </div>
                    </div>
                    <p class="import-form-tip">{{ $t('knowledgeEditor.faqImport.fileTip') }}</p>
                  </div>
                </div>

                <!-- 预览区域 -->
                <div v-if="importState.preview.length" class="import-preview">
                  <div class="preview-header">
                    <t-icon name="file-view" size="16px" class="preview-icon" />
                    <span class="preview-title">
                      {{ $t('knowledgeEditor.faqImport.previewCount', { count: importState.preview.length }) }}
                    </span>
                  </div>
                  <div class="preview-list">
                    <div
                      v-for="(item, index) in importState.preview.slice(0, 5)"
                      :key="index"
                      class="preview-item"
                    >
                      <span class="preview-index">{{ index + 1 }}</span>
                      <span class="preview-question">{{ item.standard_question }}</span>
                    </div>
                  </div>
                  <p v-if="importState.preview.length > 5" class="preview-more">
                    {{ $t('knowledgeEditor.faqImport.previewMore', { count: importState.preview.length - 5 }) }}
                  </p>
                </div>
              </div>

              <div class="faq-import-footer">
                <t-button theme="default" variant="outline" @click="importVisible = false">
                  {{ $t('common.cancel') }}
                </t-button>
                <t-button theme="primary" @click="handleImport" :loading="importState.importing">
                  {{ $t('knowledgeEditor.faqImport.importButton') }}
                </t-button>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Search Test Drawer -->
    <t-drawer
      v-model:visible="searchDrawerVisible"
      :header="$t('knowledgeEditor.faq.searchTestTitle')"
      :close-btn="true"
      size="420px"
      placement="right"
      class="faq-search-drawer"
    >
      <div class="search-test-content">
        <t-form layout="vertical" class="search-form">
          <t-form-item :label="$t('knowledgeEditor.faq.queryLabel')" class="form-item-compact">
            <t-input
              v-model="searchForm.query"
              :placeholder="$t('knowledgeEditor.faq.queryPlaceholder')"
              @keydown.enter.prevent="handleSearch"
            />
          </t-form-item>

          <t-form-item :label="$t('knowledgeEditor.faq.similarityThresholdLabel')" class="form-item-compact">
            <div class="slider-wrapper">
              <t-slider
                v-model="searchForm.vectorThreshold"
                :min="0"
                :max="1"
                :step="0.1"
                :show-tooltip="true"
                :format-tooltip="(val: number) => val.toFixed(2)"
              />
              <div class="slider-value">{{ searchForm.vectorThreshold.toFixed(2) }}</div>
            </div>
            <div class="form-tip">{{ $t('knowledgeEditor.faq.vectorThresholdDesc') }}</div>
          </t-form-item>

          <t-form-item :label="$t('knowledgeEditor.faq.matchCountLabel')" class="form-item-compact">
            <div class="slider-wrapper">
              <t-slider
                v-model="searchForm.matchCount"
                :min="1"
                :max="50"
                :step="1"
                :show-tooltip="true"
              />
              <div class="slider-value">{{ searchForm.matchCount }}</div>
            </div>
            <div class="form-tip">{{ $t('knowledgeEditor.faq.matchCountDesc') }}</div>
          </t-form-item>

          <t-form-item class="form-item-compact">
            <t-button
              theme="primary"
              block
              size="large"
              :loading="searching"
              @click="handleSearch"
              class="search-button"
            >
              {{ searching ? $t('knowledgeEditor.faq.searching') : $t('knowledgeEditor.faq.searchButton') }}
            </t-button>
          </t-form-item>
        </t-form>

        <!-- Search Results -->
        <div v-if="searchResults.length > 0 || hasSearched" class="search-results">
          <div class="results-header">
            <t-icon name="file-view" size="16px" />
            <span>{{ $t('knowledgeEditor.faq.searchResults') }} ({{ searchResults.length }})</span>
          </div>
          <div v-if="searchResults.length === 0" class="no-results">
            {{ $t('knowledgeEditor.faq.noResults') }}
          </div>
          <div v-else class="results-list">
            <div
              v-for="result in searchResults"
              :key="result.id"
              class="result-card"
              :class="{ 'expanded': result.expanded }"
            >
              <div class="result-header" @click="toggleResult(result)">
                <div class="result-question-wrapper">
                  <div class="result-question">{{ result.standard_question }}</div>
                  <t-icon 
                    :name="result.expanded ? 'chevron-up' : 'chevron-down'" 
                    class="expand-icon"
                  />
                </div>
                <div class="result-meta">
                  <t-tag size="small" variant="light-outline" class="score-tag">
                    {{ $t('knowledgeEditor.faq.score') }}: {{ (result.score || 0).toFixed(3) }}
                  </t-tag>
                </div>
              </div>
              <Transition name="slide-down">
                <div v-if="result.expanded" class="result-body">
                  <div v-if="result.answers?.length" class="result-section">
                    <div class="section-label">{{ $t('knowledgeEditor.faq.answers') }}</div>
                    <div class="result-tags">
                      <t-tooltip
                        v-for="answer in result.answers"
                        :key="answer"
                        :content="answer"
                        placement="top"
                      >
                        <t-tag size="small" theme="success" variant="light" class="answer-tag">
                          {{ answer }}
                        </t-tag>
                      </t-tooltip>
                    </div>
                  </div>
                  <div v-if="result.similar_questions?.length" class="result-section">
                    <div class="section-label">{{ $t('knowledgeEditor.faq.similarQuestions') }}</div>
                    <div class="result-tags">
                      <t-tooltip
                        v-for="question in result.similar_questions"
                        :key="question"
                        :content="question"
                        placement="top"
                      >
                        <t-tag size="small" variant="light-outline" class="question-tag">
                          {{ question }}
                        </t-tag>
                      </t-tooltip>
                    </div>
                  </div>
                </div>
              </Transition>
            </div>
          </div>
        </div>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, onMounted, computed, nextTick } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { FormRules, FormInstanceFunctions } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  listFAQEntries,
  upsertFAQEntries,
  updateFAQEntry,
  deleteFAQEntries,
  searchFAQEntries,
} from '@/api/knowledge-base'
import * as XLSX from 'xlsx'
import FAQTagTooltip from '@/components/FAQTagTooltip.vue'

interface FAQEntry {
  id: string
  chunk_id: string
  knowledge_id: string
  knowledge_base_id: string
  standard_question: string
  similar_questions: string[]
  negative_questions: string[]
  answers: string[]
  updated_at: string
  showMore?: boolean
  score?: number
  match_type?: string
  expanded?: boolean
  similarCollapsed?: boolean
  negativeCollapsed?: boolean
  answersCollapsed?: boolean
}

interface FAQEntryPayload {
  standard_question: string
  similar_questions: string[]
  negative_questions: string[]
  answers: string[]
}

const props = defineProps<{
  kbId: string
}>()

const { t } = useI18n()

const loading = ref(false)
const loadingMore = ref(false)
const entries = ref<FAQEntry[]>([])
const selectedRowKeys = ref<string[]>([])
const scrollContainer = ref<HTMLElement | null>(null)
const cardListRef = ref<HTMLElement | null>(null)
const hasMore = ref(true)
const pageSize = 20
let currentPage = 1

const editorVisible = ref(false)
const editorMode = ref<'create' | 'edit'>('create')
const currentEntryId = ref<string | null>(null)
const editorForm = reactive<FAQEntryPayload>({
  standard_question: '',
  similar_questions: [],
  negative_questions: [],
  answers: [],
})
const editorFormRef = ref<FormInstanceFunctions>()
const savingEntry = ref(false)

// 输入框状态
const answerInput = ref('')
const similarInput = ref('')
const negativeInput = ref('')

const importVisible = ref(false)
const fileInputRef = ref<HTMLInputElement | null>(null)
const importState = reactive({
  mode: 'append' as 'append' | 'replace',
  file: null as File | null,
  preview: [] as FAQEntryPayload[],
  importing: false,
})

// Search test state
const searchDrawerVisible = ref(false)
const searching = ref(false)
const hasSearched = ref(false)
const searchResults = ref<FAQEntry[]>([])
const searchForm = reactive({
  query: '',
  vectorThreshold: 0.7,
  matchCount: 10,
})

// Toolbar actions dropdown
const toolbarActionOptions = computed(() => {
  const options = [
    { content: t('knowledgeEditor.faqImport.importButton'), value: 'import', icon: 'upload' },
    { content: t('knowledgeEditor.faq.searchTest'), value: 'search', icon: 'search' },
  ]
  
  // 如果有选中的条目，添加批量删除选项
  if (selectedRowKeys.value.length > 0) {
    options.push({
      content: `${t('knowledgeEditor.faqImport.deleteSelected')} (${selectedRowKeys.value.length})`,
      value: 'delete',
      icon: 'delete',
    })
  }
  
  return options
})

const handleToolbarAction = (data: { value: string }) => {
  switch (data.value) {
    case 'import':
      openImportDialog()
      break
    case 'search':
      searchDrawerVisible.value = true
      break
    case 'delete':
      handleBatchDelete()
      break
  }
}

const editorRules: FormRules<FAQEntryPayload> = {
  standard_question: [
    { required: true, message: t('knowledgeEditor.messages.nameRequired') },
  ],
  answers: [
    {
      validator: (val: string[]) => Array.isArray(val) && val.length > 0,
      message: t('knowledgeEditor.faq.answerRequired'),
    },
  ],
}

const loadEntries = async (append = false) => {
  if (!props.kbId) return
  if (append) {
    loadingMore.value = true
  } else {
    loading.value = true
    currentPage = 1
    entries.value = []
    selectedRowKeys.value = []
  }

  try {
    const res = await listFAQEntries(props.kbId, {
      page: currentPage,
      page_size: pageSize,
    })
    const pageData = (res.data || {}) as {
      data: FAQEntry[]
      total: number
    }
    const newEntries = (pageData.data || []).map(entry => ({
      ...entry,
      showMore: false,
      similarCollapsed: false, // 相似问默认展开
      negativeCollapsed: true,  // 反例默认折叠
      answersCollapsed: true    // 答案默认折叠
    }))
    
    if (append) {
      entries.value = [...entries.value, ...newEntries]
    } else {
      entries.value = newEntries
    }

    // 判断是否还有更多数据
    hasMore.value = entries.value.length < (pageData.total || 0)
    currentPage++
    
    // 等待 DOM 更新后重新布局
    await nextTick()
    arrangeCards()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('common.operationFailed'))
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}

const handleScroll = () => {
  if (!scrollContainer.value || loadingMore.value || !hasMore.value) return

  const container = scrollContainer.value
  const scrollTop = container.scrollTop
  const scrollHeight = container.scrollHeight
  const clientHeight = container.clientHeight

  // 当滚动到距离底部 200px 时加载更多
  if (scrollTop + clientHeight >= scrollHeight - 200) {
    loadEntries(true)
  }
}

const handleCardSelect = (entryId: string, checked: boolean) => {
  if (checked) {
    if (!selectedRowKeys.value.includes(entryId)) {
      selectedRowKeys.value.push(entryId)
    }
  } else {
    const index = selectedRowKeys.value.indexOf(entryId)
    if (index > -1) {
      selectedRowKeys.value.splice(index, 1)
    }
  }
}

const resetEditorForm = () => {
  editorForm.standard_question = ''
  editorForm.similar_questions = []
  editorForm.negative_questions = []
  editorForm.answers = []
  answerInput.value = ''
  similarInput.value = ''
  negativeInput.value = ''
}

const openEditor = (entry?: FAQEntry) => {
  if (entry) {
    editorMode.value = 'edit'
    currentEntryId.value = entry.id
    editorForm.standard_question = entry.standard_question
    editorForm.similar_questions = [...(entry.similar_questions || [])]
    editorForm.negative_questions = [...(entry.negative_questions || [])]
    editorForm.answers = [...(entry.answers || [])]
  } else {
    editorMode.value = 'create'
    currentEntryId.value = null
    resetEditorForm()
  }
  answerInput.value = ''
  similarInput.value = ''
  negativeInput.value = ''
  editorVisible.value = true
}

const handleEditorClose = () => {
  // 关闭时重置表单
  resetEditorForm()
  answerInput.value = ''
  similarInput.value = ''
  negativeInput.value = ''
  editorFormRef.value?.clearValidate?.()
}

// 添加答案
const addAnswer = () => {
  const trimmed = answerInput.value.trim()
  if (trimmed && editorForm.answers.length < 5 && !editorForm.answers.includes(trimmed)) {
    editorForm.answers.push(trimmed)
    answerInput.value = ''
  }
}

// 删除答案
const removeAnswer = (index: number) => {
  editorForm.answers.splice(index, 1)
}

// 添加相似问
const addSimilar = () => {
  const trimmed = similarInput.value.trim()
  if (trimmed && editorForm.similar_questions.length < 10 && !editorForm.similar_questions.includes(trimmed)) {
    editorForm.similar_questions.push(trimmed)
    similarInput.value = ''
  }
}

// 删除相似问
const removeSimilar = (index: number) => {
  editorForm.similar_questions.splice(index, 1)
}

// 添加反例
const addNegative = () => {
  const trimmed = negativeInput.value.trim()
  if (trimmed && editorForm.negative_questions.length < 10 && !editorForm.negative_questions.includes(trimmed)) {
    editorForm.negative_questions.push(trimmed)
    negativeInput.value = ''
  }
}

// 删除反例
const removeNegative = (index: number) => {
  editorForm.negative_questions.splice(index, 1)
}

const handleSubmitEntry = async () => {
  if (!editorFormRef.value) return
  const result = await editorFormRef.value.validate?.()
  if (result !== true) return

  savingEntry.value = true
  try {
    if (editorMode.value === 'create') {
      await upsertFAQEntries(props.kbId, {
        entries: [editorForm],
        mode: 'append',
      })
      MessagePlugin.success(t('knowledgeEditor.messages.createSuccess'))
    } else if (currentEntryId.value) {
      await updateFAQEntry(props.kbId, currentEntryId.value, editorForm)
      MessagePlugin.success(t('knowledgeEditor.messages.updateSuccess'))
    }
    editorVisible.value = false
    await loadEntries()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('common.operationFailed'))
  } finally {
    savingEntry.value = false
  }
}

const handleBatchDelete = async () => {
  if (!selectedRowKeys.value.length) return
  try {
    await deleteFAQEntries(props.kbId, selectedRowKeys.value)
    MessagePlugin.success(t('knowledgeEditor.faqImport.deleteSuccess'))
    await loadEntries()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('common.operationFailed'))
  }
}

const handleMenuEdit = (entry: FAQEntry) => {
  entry.showMore = false
  openEditor(entry)
}

const handleMenuDelete = async (entry: FAQEntry) => {
  entry.showMore = false
  try {
    await deleteFAQEntries(props.kbId, [entry.id])
    MessagePlugin.success(t('knowledgeEditor.faqImport.deleteSuccess'))
    await loadEntries()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('common.operationFailed'))
  }
}

const openImportDialog = () => {
  importVisible.value = true
  importState.file = null
  importState.preview = []
  importState.mode = 'append'
}

const processFile = async (file: File) => {
  importState.file = file

  try {
    let parsed: FAQEntryPayload[] = []
    if (file.name.endsWith('.json')) {
      parsed = await parseJSONFile(file)
    } else if (file.name.endsWith('.csv')) {
      parsed = await parseCSVFile(file)
    } else if (file.name.endsWith('.xlsx') || file.name.endsWith('.xls')) {
      parsed = await parseExcelFile(file)
    } else {
      MessagePlugin.warning(t('knowledgeEditor.faqImport.unsupportedFormat'))
      importState.preview = []
      return
    }
    importState.preview = parsed
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('knowledgeEditor.faqImport.parseFailed'))
    importState.preview = []
  }
}

const handleFileChange = async (event: Event) => {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  await processFile(file)
}

const handleFileDrop = async (event: DragEvent) => {
  const file = event.dataTransfer?.files[0]
  if (!file) return
  await processFile(file)
}

const parseJSONFile = async (file: File): Promise<FAQEntryPayload[]> => {
  const text = await file.text()
  const data = JSON.parse(text)
  if (!Array.isArray(data)) {
    throw new Error(t('knowledgeEditor.faqImport.invalidJSON'))
  }
  return data.map(normalizePayload)
}

const parseCSVFile = async (file: File): Promise<FAQEntryPayload[]> => {
  const text = await file.text()
  const [headerLine, ...rows] = text.split(/\r?\n/).filter(Boolean)
  const headers = headerLine.split(',').map((h) => h.trim().toLowerCase())
  const payloads: FAQEntryPayload[] = []
  rows.forEach((line) => {
    const columns = line.split(',').map((c) => c.trim())
    const record: Record<string, string> = {}
    headers.forEach((key, idx) => {
      record[key] = columns[idx] || ''
    })
    payloads.push(
      normalizePayload({
        standard_question: record['standard_question'] || record['question'] || '',
        answers: splitByDelimiter(record['answers']),
        similar_questions: splitByDelimiter(record['similar_questions']),
        negative_questions: splitByDelimiter(record['negative_questions']),
      }),
    )
  })
  return payloads
}

const parseExcelFile = async (file: File): Promise<FAQEntryPayload[]> => {
  const data = await file.arrayBuffer()
  const workbook = XLSX.read(data, { type: 'array' })
  const sheetName = workbook.SheetNames[0]
  const worksheet = workbook.Sheets[sheetName]
  const json = XLSX.utils.sheet_to_json<Record<string, string>>(worksheet, { defval: '' })
  return json.map((row) =>
    normalizePayload({
      standard_question: row['standard_question'] || row['question'] || '',
      answers: splitByDelimiter(row['answers']),
      similar_questions: splitByDelimiter(row['similar_questions']),
      negative_questions: splitByDelimiter(row['negative_questions']),
    }),
  )
}

const splitByDelimiter = (value?: string) => {
  if (!value) return []
  // 支持引号包裹的内容，避免包含分隔符的内容被错误分割
  const result: string[] = []
  let current = ''
  let inQuotes = false
  // 支持多种引号字符
  const quoteChars = ['"', "'", '\u201C', '\u201D', '\u2018', '\u2019', '\u300C', '\u300D', '\u300E', '\u300F']
  
  for (let i = 0; i < value.length; i++) {
    const char = value[i]
    
    // 检查是否是引号
    if (quoteChars.includes(char)) {
      inQuotes = !inQuotes
      continue
    }
    
    // 如果在引号内，直接添加到当前字符串
    if (inQuotes) {
      current += char
      continue
    }
    
    // 检查是否是分隔符
    if (/[\n;；,，]/.test(char)) {
      const trimmed = current.trim()
      if (trimmed) {
        result.push(trimmed)
      }
      current = ''
    } else {
      current += char
    }
  }
  
  // 添加最后一部分
  const trimmed = current.trim()
  if (trimmed) {
    result.push(trimmed)
  }
  
  return result.filter(Boolean)
}

const normalizePayload = (payload: Partial<FAQEntryPayload>): FAQEntryPayload => ({
  standard_question: payload.standard_question || '',
  answers: payload.answers?.filter(Boolean) || [],
  similar_questions: payload.similar_questions?.filter(Boolean) || [],
  negative_questions: payload.negative_questions?.filter(Boolean) || [],
})

const handleImport = async () => {
  if (!importState.file || !importState.preview.length) {
    MessagePlugin.warning(t('knowledgeEditor.faqImport.selectFile'))
    return
  }
  importState.importing = true
  try {
    await upsertFAQEntries(props.kbId, {
      entries: importState.preview,
      mode: importState.mode,
    })
    MessagePlugin.success(t('knowledgeEditor.faqImport.importSuccess'))
    importVisible.value = false
    await loadEntries()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('common.operationFailed'))
  } finally {
    importState.importing = false
  }
}

watch(
  () => props.kbId,
  () => {
    currentPage = 1
    hasMore.value = true
    loadEntries()
  },
  { immediate: true },
)

const handleSearch = async () => {
  if (!searchForm.query.trim()) {
    MessagePlugin.warning(t('knowledgeEditor.faq.queryPlaceholder'))
    return
  }

  searching.value = true
  hasSearched.value = true
  try {
    const res = await searchFAQEntries(props.kbId, {
      query_text: searchForm.query.trim(),
      vector_threshold: searchForm.vectorThreshold,
      match_count: searchForm.matchCount,
    })
    searchResults.value = (res.data || []).map((entry: FAQEntry) => ({
      ...entry,
      similarCollapsed: false, // 相似问默认展开
      negativeCollapsed: true,  // 反例默认折叠
      answersCollapsed: true,   // 答案默认折叠
      expanded: false,
    })) as FAQEntry[]
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('common.operationFailed'))
    searchResults.value = []
  } finally {
    searching.value = false
  }
}

const getMatchTypeLabel = (matchType?: string) => {
  if (!matchType) return ''
  if (matchType === 'embedding') {
    return t('knowledgeEditor.faq.matchTypeEmbedding')
  }
  if (matchType === 'keywords') {
    return t('knowledgeEditor.faq.matchTypeKeywords')
  }
  return matchType
}

const toggleResult = (result: FAQEntry) => {
  result.expanded = !result.expanded
}

// 瀑布流布局函数
const arrangeCards = () => {
  if (!cardListRef.value) return
  
  const cards = cardListRef.value.querySelectorAll('.faq-card') as NodeListOf<HTMLElement>
  if (cards.length === 0) return
  
  // 获取容器宽度和列数
  const containerWidth = cardListRef.value.offsetWidth
  const gap = 12 // 与 CSS gap 保持一致
  let columnCount = 1
  
  // 根据容器宽度计算列数（增加每行的卡片数量）
  if (containerWidth >= 2560) columnCount = 12
  else if (containerWidth >= 1920) columnCount = 10
  else if (containerWidth >= 1536) columnCount = 8
  else if (containerWidth >= 1280) columnCount = 6
  else if (containerWidth >= 1024) columnCount = 5
  else if (containerWidth >= 768) columnCount = 4
  else if (containerWidth >= 640) columnCount = 3
  
  const columnWidth = (containerWidth - (gap * (columnCount - 1))) / columnCount
  
  // 初始化每列的高度数组
  const columnHeights = new Array(columnCount).fill(0)
  
  // 先重置所有卡片的位置，让它们自然排列以获取正确的高度
  cards.forEach((card) => {
    card.style.position = 'static'
    card.style.top = 'auto'
    card.style.left = 'auto'
    card.style.width = 'auto'
  })
  
  // 等待浏览器重新计算布局
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      cards.forEach((card) => {
        // 找到最短的列
        const shortestColumnIndex = columnHeights.indexOf(Math.min(...columnHeights))
        
        // 设置卡片位置
        const top = columnHeights[shortestColumnIndex]
        const left = shortestColumnIndex * (columnWidth + gap)
        
        card.style.position = 'absolute'
        card.style.top = `${top}px`
        card.style.left = `${left}px`
        card.style.width = `${columnWidth}px`
        
        // 更新该列的高度（使用实际高度）
        const cardHeight = card.offsetHeight || card.getBoundingClientRect().height
        columnHeights[shortestColumnIndex] += cardHeight + gap
      })
      
      // 设置容器高度
      const maxHeight = Math.max(...columnHeights)
      if (cardListRef.value) {
        cardListRef.value.style.height = `${maxHeight}px`
        cardListRef.value.style.position = 'relative'
      }
    })
  })
}

// 监听窗口大小变化
const handleResize = () => {
  arrangeCards()
}

onMounted(() => {
  if (props.kbId) {
    loadEntries()
  }
  window.addEventListener('resize', handleResize)
})

// 监听 entries 变化，重新布局
watch(() => entries.value.length, () => {
  nextTick(() => {
    arrangeCards()
  })
})

// 监听折叠状态变化，重新布局（包括展开和收起）
watch(() => entries.value.map(e => ({
  id: e.id,
  similarCollapsed: e.similarCollapsed,
  negativeCollapsed: e.negativeCollapsed,
  answersCollapsed: e.answersCollapsed
})), () => {
  // 使用双重 nextTick 确保 DOM 完全更新后再布局
  nextTick(() => {
    nextTick(() => {
      // 等待 Transition 动画完成（slide-down 动画时长约 300ms）
      setTimeout(() => {
        arrangeCards()
      }, 350)
    })
  })
}, { deep: true })
</script>

<style scoped lang="less">
.faq-manager {
  display: flex;
  flex-direction: column;
  height: 100%;
}

// Header 样式
.faq-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
  flex-shrink: 0;

  .faq-header-title {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  h2 {
    margin: 0;
    color: #000000e6;
    font-family: "PingFang SC";
    font-size: 24px;
    font-weight: 600;
    line-height: 32px;
  }

  .faq-subtitle {
    margin: 0;
    color: #00000099;
    font-family: "PingFang SC";
    font-size: 14px;
    font-weight: 400;
    line-height: 20px;
  }
}

.action-buttons {
  display: flex;
  gap: 12px;
  align-items: center;
}

.create-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 20px;
  height: 36px;
  border: 1px solid transparent;
  border-radius: 8px;
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
  background: transparent;

  .btn-icon {
    flex-shrink: 0;
  }
}

.create-btn.ghost {
  background: transparent;
  color: #07c05f;
  border-color: #07c05f;

  &:hover {
    background: #07c05f1a;
  }

  &:active {
    background: #07c05f33;
  }
}

.toolbar-action-trigger {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: 1px solid #d9d9d9;
  border-radius: 8px;
  background: #ffffff;
  cursor: pointer;
  transition: all 0.2s ease;
  color: #00000099;

  &:hover {
    background-color: #f5f5f5;
    border-color: #07c05f;
    color: #07c05f;
  }

  :deep(.t-icon) {
    font-size: 16px;
  }
}

// 滚动容器
.faq-scroll-container {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 4px;
}

// 卡片列表样式 - 使用绝对定位实现瀑布流，下一行补齐上一行空缺
.faq-card-list {
  position: relative;
  width: 100%;
  min-width: 0;
}

.faq-card {
  border: 1px solid #E7E7E7;
  border-radius: 12px;
  background: #fff;
  padding: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  cursor: pointer;
  transition: all 0.2s ease;
  box-sizing: border-box;
  height: fit-content; // 高度根据内容自适应

  &:hover {
    border-color: #07C05F;
    box-shadow: 0 2px 8px rgba(7, 192, 95, 0.1);
  }

  &.selected {
    border-color: #07C05F;
    background: #F0FDF4;
    box-shadow: 0 2px 8px rgba(7, 192, 95, 0.15);
  }
}

.faq-card-header {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid #F3F4F6;
  position: relative;
}

.card-more-btn {
  display: flex;
  width: 28px;
  height: 28px;
  justify-content: center;
  align-items: center;
  border-radius: 6px;
  cursor: pointer;
  flex-shrink: 0;
  margin-left: auto;
  opacity: 0.6;

  &:hover {
    background: #F3F4F6;
    opacity: 1;
  }

  .more-icon {
    width: 16px;
    height: 16px;
  }
}

.card-menu {
  display: flex;
  flex-direction: column;
  min-width: 140px;
  padding: 4px;
  border-radius: 8px;
}

.card-menu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  cursor: pointer;
  color: #111827;
  font-family: "PingFang SC";
  font-size: 14px;
  border-radius: 6px;

  &:hover {
    background: #F3F4F6;
    color: #07C05F;
  }

  &.danger {
    color: #EF4444;

    &:hover {
      background: #FEE2E2;
      color: #DC2626;
    }
  }

  .menu-icon {
    font-size: 16px;
    flex-shrink: 0;
  }
}

.faq-question {
  flex: 1;
  color: #111827;
  font-family: "PingFang SC";
  font-size: 15px;
  font-weight: 600;
  line-height: 1.5;
  word-break: break-word;
  min-width: 0;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  line-clamp: 2;
  -webkit-box-orient: vertical;
}

.faq-card-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.faq-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  overflow: hidden;

  .faq-section-label {
    color: #6B7280;
    font-family: "PingFang SC";
    font-size: 12px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 2px;

    &::before {
      content: '';
      width: 3px;
      height: 12px;
      background: #07C05F;
      border-radius: 2px;
      flex-shrink: 0;
    }

    &.clickable {
      cursor: pointer;
      user-select: none;
      padding: 2px 0;
      border-radius: 4px;

      &:hover {
        color: #111827;
        background: #F9FAFB;
        padding-left: 4px;
        padding-right: 4px;
        margin-left: -4px;
        margin-right: -4px;
      }
    }

    .collapse-icon {
      font-size: 14px;
      color: #9CA3AF;
      flex-shrink: 0;
      margin-left: auto; // 让箭头靠右对齐
    }

    .section-count {
      color: #9CA3AF;
      font-weight: 400;
      margin-left: 4px;
    }
  }

  &.answers .faq-section-label::before {
    background: #07C05F;
  }

  &.similar .faq-section-label::before {
    background: #3B82F6;
  }

  &.negative .faq-section-label::before {
    background: #F59E0B;
  }
}

.faq-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  min-height: 20px;
  min-width: 0;
  width: 100%;
  overflow: hidden;
  
  // 确保每个标签都有最大宽度限制
  > * {
    max-width: 100%;
    min-width: 0;
    flex: 0 1 auto;
  }
  
  // 当标签单独一行时，限制最大宽度
  > *:first-child:last-child {
    max-width: 100%;
  }
}

.question-tag {
  font-size: 12px;
  padding: 4px 10px;
  max-width: 100%;
  min-width: 0;
  border-radius: 6px;
  font-family: "PingFang SC";
  flex: 0 1 auto;
  
  :deep(.t-tag) {
    max-width: 100% !important;
    min-width: 0 !important;
    width: auto !important;
    display: inline-flex !important;
    align-items: center;
    vertical-align: middle;
    overflow: hidden !important;
    box-sizing: border-box;
    background: #F9FAFB;
    border-color: #E5E7EB;
    color: #374151;
  }
  
  // 针对TDesign tag内部的span元素
  :deep(.t-tag span),
  :deep(.t-tag > span) {
    display: block !important;
    overflow: hidden !important;
    text-overflow: ellipsis !important;
    white-space: nowrap !important;
    max-width: 100% !important;
    width: auto !important;
    line-height: 1.4;
    min-width: 0 !important;
  }
}

// 确保 tag 本身不会超出容器
.faq-tags :deep(.t-tag) {
  max-width: 100%;
  min-width: 0;
  flex-shrink: 1;
}

.faq-tags :deep(.faq-tag-wrapper) {
  max-width: 100%;
  min-width: 0;
  flex-shrink: 1;
}

.empty-tip {
  color: #9CA3AF;
  font-size: 12px;
  font-style: italic;
  padding: 8px 0;
  font-family: "PingFang SC";
}


.faq-load-more,
.faq-no-more {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 24px 16px;
  color: #6B7280;
  font-size: 13px;
  font-family: "PingFang SC";
}

.faq-no-more {
  color: #9CA3AF;
  font-style: italic;
}

// 空状态样式
.faq-empty-state {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 400px;
  padding: 60px 20px;

  .empty-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
    text-align: center;
    max-width: 400px;
  }

  .empty-icon {
    color: #D1D5DB;
    opacity: 0.6;
  }

  .empty-text {
    color: #111827;
    font-family: "PingFang SC";
    font-size: 18px;
    font-weight: 600;
    line-height: 28px;
  }

  .empty-desc {
    color: #6B7280;
    font-family: "PingFang SC";
    font-size: 14px;
    font-weight: 400;
    line-height: 22px;
  }
}

// 导入对话框样式 - 与创建知识库弹窗风格一致
.faq-import-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  backdrop-filter: blur(4px);
}

.faq-import-modal {
  position: relative;
  width: 100%;
  max-width: 600px;
  max-height: 90vh;
  background: #ffffff;
  border-radius: 12px;
  box-shadow: 0 6px 28px rgba(15, 23, 42, 0.08);
  overflow: hidden;
  display: flex;
  flex-direction: column;

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
}

.faq-import-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.faq-import-header {
  padding: 24px 24px 16px;
  border-bottom: 1px solid #e5e5e5;
  flex-shrink: 0;

  .import-title {
    margin: 0;
    font-family: "PingFang SC";
    font-size: 18px;
    font-weight: 600;
    color: #000000e6;
  }
}

.faq-import-content {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 24px;
  min-height: 0;
  max-height: calc(90vh - 140px); // 减去 header 和 footer 的高度
  
  // 自定义滚动条
  &::-webkit-scrollbar {
    width: 6px;
  }

  &::-webkit-scrollbar-track {
    background: #f5f5f5;
    border-radius: 3px;
  }

  &::-webkit-scrollbar-thumb {
    background: #d0d0d0;
    border-radius: 3px;
    transition: background 0.2s;

    &:hover {
      background: #07C05F;
    }
  }
}

.faq-import-footer {
  padding: 16px 24px;
  border-top: 1px solid #e5e5e5;
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  flex-shrink: 0;
}

// 导入表单项
.import-form-item {
  margin-bottom: 24px;

  &:last-child {
    margin-bottom: 0;
  }
}

// 导入表单标签
.import-form-label {
  display: block;
  margin-bottom: 10px;
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 500;
  color: #333333;
  letter-spacing: -0.2px;

  &.required::after {
    content: '*';
    color: #FA5151;
    margin-left: 4px;
    font-weight: 600;
  }
}

// 单选按钮组样式
:deep(.import-radio-group) {
  .t-radio-button {
    font-family: "PingFang SC";
    font-size: 14px;
    transition: all 0.2s ease;

    &:hover {
      border-color: #07C05F;
    }

    &.t-is-checked {
      background-color: #07C05F;
      border-color: #07C05F;
      color: #ffffff;
    }
  }
}

// 文件上传包装器
.file-upload-wrapper {
  width: 100%;
}

// 隐藏的文件输入
.file-input-hidden {
  position: absolute;
  width: 0;
  height: 0;
  opacity: 0;
  overflow: hidden;
  pointer-events: none;
}

// 文件上传区域
.file-upload-area {
  position: relative;
  width: 100%;
  min-height: 120px;
  border: 2px dashed #d9d9d9;
  border-radius: 8px;
  background: #fafafa;
  cursor: pointer;
  transition: all 0.3s ease;
  display: flex;
  align-items: center;
  justify-content: center;

  &:hover {
    border-color: #07C05F;
    background: #f0fdf4;
  }

  &.has-file {
    border-color: #07C05F;
    background: #f0fdf4;
    border-style: solid;
  }
}

// 文件上传内容
.file-upload-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  text-align: center;
}

.upload-icon {
  color: #07C05F;
  transition: transform 0.2s ease;
}

.file-upload-area:hover .upload-icon {
  transform: translateY(-2px);
}

.upload-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.upload-primary-text {
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 500;
  color: #333333;
}

.upload-secondary-text {
  font-family: "PingFang SC";
  font-size: 12px;
  color: #666666;
}

.upload-file-name {
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 500;
  color: #07C05F;
  word-break: break-all;
}

// 导入表单提示
.import-form-tip {
  margin-top: 8px;
  font-family: "PingFang SC";
  font-size: 12px;
  color: #00000066;
  line-height: 18px;
}

// 预览区域
.import-preview {
  margin-top: 20px;
  padding: 16px;
  background: #fafafa;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.preview-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid #e5e7eb;
}

.preview-icon {
  color: #07C05F;
  flex-shrink: 0;
}

.preview-title {
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 500;
  color: #333333;
}

.preview-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 8px;
}

.preview-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 12px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  transition: all 0.2s ease;

  &:hover {
    border-color: #07C05F;
    box-shadow: 0 2px 4px rgba(7, 192, 95, 0.08);
  }
}

.preview-index {
  flex-shrink: 0;
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #07C05F 0%, #05a04f 100%);
  color: #ffffff;
  border-radius: 4px;
  font-family: "PingFang SC";
  font-size: 12px;
  font-weight: 600;
}

.preview-question {
  flex: 1;
  font-family: "PingFang SC";
  font-size: 13px;
  color: #333333;
  line-height: 1.5;
  word-break: break-word;
}

.preview-more {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid #e5e7eb;
  font-family: "PingFang SC";
  font-size: 12px;
  color: #666666;
  text-align: center;
}

// 响应式布局由 JavaScript 动态计算，这里不需要媒体查询

// 卡片菜单弹窗样式
:deep(.faq-card-popup) {
  z-index: 99 !important;

  .t-popup__content {
    padding: 4px 0 !important;
    margin-top: 4px !important;
    min-width: 120px;
  }
}

// FAQ 编辑器抽屉样式
:deep(.faq-editor-drawer) {
  .t-drawer__body {
    padding: 20px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .t-drawer__header {
    padding: 20px 24px;
    border-bottom: 1px solid #e5e5e5;
    font-family: "PingFang SC";
    font-size: 18px;
    font-weight: 600;
    color: #000000e6;
  }

  .t-drawer__footer {
    padding: 16px 24px;
    border-top: 1px solid #e5e5e5;
  }
}

.faq-editor-drawer-content {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  min-height: 0;
  
  // 自定义滚动条
  &::-webkit-scrollbar {
    width: 6px;
  }

  &::-webkit-scrollbar-track {
    background: #f5f5f5;
    border-radius: 3px;
  }

  &::-webkit-scrollbar-thumb {
    background: #d0d0d0;
    border-radius: 3px;
    transition: background 0.2s;

    &:hover {
      background: #07C05F;
    }
  }

  .editor-form {
    width: 100%;
  }
}

.faq-editor-drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

// 全宽输入框包装器 - 统一样式
.full-width-input-wrapper {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;

  .full-width-input {
    flex: 1;
  }

  .full-width-textarea {
    flex: 1;
    
    :deep(.t-textarea__inner) {
      min-height: 80px;
    }
  }

  // textarea需要顶部对齐
  &.textarea-wrapper {
    align-items: flex-start;
  }

  .add-item-btn {
    flex-shrink: 0;
    width: 32px;
    height: 32px;
    min-width: 32px;
    padding: 0;
    font-family: "PingFang SC";
    transition: all 0.2s ease;
    border-radius: 8px;
  }

  :deep(.add-item-btn) {
    background: #07C05F !important;
    border: 1px solid #07C05F !important;
    border-radius: 8px !important;
    color: #ffffff !important;
    display: flex;
    align-items: center;
    justify-content: center;

    &:hover:not(:disabled) {
      background: #05a04f !important;
      border-color: #05a04f !important;
      transform: scale(1.05);
      box-shadow: 0 2px 8px rgba(7, 192, 95, 0.3);
    }

    &:active:not(:disabled) {
      background: #048a42 !important;
      border-color: #048a42 !important;
      transform: scale(0.98);
    }

    &:disabled {
      background: #E5E7EB !important;
      border-color: #E5E7EB !important;
      color: #9CA3AF !important;
      cursor: not-allowed;
      opacity: 0.6;
    }

    .t-icon {
      font-size: 16px;
    }
  }
}

.item-count {
  font-size: 13px;
  color: #6B7280;
  font-family: "PingFang SC";
  font-weight: 500;
  margin-bottom: 12px;
  text-align: right;
  padding-right: 4px;
}

.item-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
}

.section-divider {
  height: 1px;
  background: linear-gradient(to right, transparent, #E7E7E7, transparent);
  margin: 24px 0;
  border: none;
}

.item-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  background: #ffffff;
  border: 1px solid #E7E7E7;
  border-radius: 8px;
  transition: all 0.2s ease;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
  position: relative;

  &.answer-row {
    align-items: flex-start;
    padding: 12px 14px;
  }

  &:hover {
    background: #fafafa;
    border-color: #07C05F;
    box-shadow: 0 2px 8px rgba(7, 192, 95, 0.12);
    transform: translateY(-1px);
  }

  &.negative {
    background: #FFFBEB;
    border-color: #FDE68A;

    &:hover {
      background: #FEF3C7;
      border-color: #FCD34D;
      box-shadow: 0 2px 8px rgba(251, 191, 36, 0.15);
    }
  }

  .item-content {
    flex: 1;
    font-size: 14px;
    line-height: 1.6;
    color: #111827;
    font-family: "PingFang SC";
    white-space: pre-wrap;
    word-break: break-word;
    padding: 0;
    font-weight: 400;
  }

  .remove-item-btn {
    flex-shrink: 0;
    color: #9CA3AF;
    padding: 0;
    width: 24px;
    height: 24px;
    min-width: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 6px;
    transition: all 0.2s ease;
    background: transparent;
    border: none;
    cursor: pointer;

    &:hover {
      color: #EF4444;
      background: #FEE2E2;
    }

    &:active {
      background: #FECACA;
    }

    :deep(.t-icon) {
      font-size: 14px;
    }
  }

  &.answer-row .remove-item-btn {
    margin-top: 0;
  }
}

.form-tip {
  margin-top: 6px;
  font-size: 12px;
  color: #00000066;
  font-family: "PingFang SC";
}

// 表单样式优化 - 与项目整体风格一致
:deep(.t-form) {
  .t-form__controls-content {
    margin: 0;
    display: block !important;
    align-items: unset !important;
    min-height: unset !important;
  }
}

:deep(.t-form-item) {
  margin-bottom: 24px;

  &:last-child {
    margin-bottom: 0;
  }

  .t-form-item__label {
    font-family: "PingFang SC";
    font-size: 14px;
    font-weight: 500;
    color: #111827;
    margin-bottom: 10px;
    line-height: 22px;
    display: block;
  }

  .t-form-item__content {
    margin: 0;
    width: 100%;
  }
}

// Input 组件样式 - 与登录页面一致
:deep(.t-input) {
  font-family: "PingFang SC";
  font-size: 14px;
  border: 1px solid #E7E7E7;
  border-radius: 8px;
  background: #fff;
  transition: all 0.2s ease;

  &:hover {
    border-color: #07C05F;
  }

  &:focus-within {
    border-color: #07C05F;
    box-shadow: 0 0 0 3px rgba(7, 192, 95, 0.1);
  }

  .t-input__inner {
    border: none !important;
    box-shadow: none !important;
    outline: none !important;
    background: transparent;
    font-size: 14px;
    font-family: "PingFang SC";
    padding: 8px 12px;
    color: #111827;

    &:focus {
      border: none !important;
      box-shadow: none !important;
      outline: none !important;
    }

    &::placeholder {
      color: #9CA3AF;
    }
  }

  .t-input__wrap {
    border: none !important;
    box-shadow: none !important;
  }
}

// Textarea 组件样式
:deep(.t-textarea) {
  font-family: "PingFang SC";
  font-size: 14px;
  border: 1px solid #E7E7E7;
  border-radius: 8px;
  background: #fff;
  transition: all 0.2s ease;

  &:hover {
    border-color: #07C05F;
  }

  &:focus-within {
    border-color: #07C05F;
    box-shadow: 0 0 0 3px rgba(7, 192, 95, 0.1);
  }

  .t-textarea__inner {
    border: none !important;
    box-shadow: none !important;
    outline: none !important;
    background: transparent;
    font-size: 14px;
    font-family: "PingFang SC";
    line-height: 1.6;
    resize: vertical;
    padding: 8px 12px;
    color: #111827;

    &:focus {
      border: none !important;
      box-shadow: none !important;
      outline: none !important;
    }

    &::placeholder {
      color: #9CA3AF;
    }
  }
}

:deep(.t-button--theme-primary) {
  background-color: #07c05f;
  border-color: #07c05f;
  
  &:hover {
    background-color: #05a04f;
    border-color: #05a04f;
  }
}

// 导入弹窗动画
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active .faq-import-modal,
.modal-leave-active .faq-import-modal {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .faq-import-modal,
.modal-leave-to .faq-import-modal {
  transform: scale(0.95);
  opacity: 0;
}

// Tag 样式优化
.answer-tag {
  background: #07c05f1a;
  color: #07c05f;
  border-color: #07c05f33;
}

.question-tag {
  background: #fff;
  border-color: #d9d9d9;
  color: #00000099;
}

// Search test drawer styles - 与编辑器抽屉风格一致
:deep(.faq-search-drawer) {
  .t-drawer__body {
    padding: 20px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .t-drawer__header {
    padding: 20px 24px;
    border-bottom: 1px solid #e5e5e5;
    font-family: "PingFang SC";
    font-size: 18px;
    font-weight: 600;
    color: #000000e6;
  }
}

.search-test-content {
  display: flex;
  flex-direction: column;
  gap: 20px;
  height: 100%;
}

.search-form {
  flex-shrink: 0;
}

.form-item-compact {
  margin-bottom: 20px;

  &:last-child {
    margin-bottom: 0;
  }
}

:deep(.form-item-compact .t-form-item__label) {
  margin-bottom: 10px;
  font-size: 14px;
  font-weight: 500;
  color: #111827;
  font-family: "PingFang SC";
}

:deep(.form-item-compact .t-form-item__content) {
  margin: 0;
  width: 100%;
}

.slider-wrapper {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  padding: 4px 0;
}

:deep(.slider-wrapper .t-slider) {
  flex: 1;
  min-width: 0;

  .t-slider__rail {
    background: #E7E7E7;
    height: 4px;
    border-radius: 2px;
  }

  .t-slider__track {
    background: #07C05F;
    height: 4px;
    border-radius: 2px;
  }

  .t-slider__button {
    width: 16px;
    height: 16px;
    border: 2px solid #07C05F;
    background: #ffffff;
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);

    &:hover {
      border-color: #05a04f;
      box-shadow: 0 2px 8px rgba(7, 192, 95, 0.2);
    }
  }
}

.slider-value {
  flex-shrink: 0;
  min-width: 50px;
  text-align: right;
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 500;
  color: #111827;
  padding: 4px 8px;
  background: #F9FAFB;
  border-radius: 6px;
}

.form-tip {
  margin-top: 8px;
  font-size: 12px;
  color: #6B7280;
  font-family: "PingFang SC";
  line-height: 18px;
}

.search-button {
  height: 40px;
  border-radius: 8px;
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 500;
  transition: all 0.2s ease;

  &:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(7, 192, 95, 0.3);
  }

  &:active:not(:disabled) {
    transform: translateY(0);
  }
}

.search-results {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-top: 1px solid #E7E7E7;
  padding-top: 20px;
  width: 100%;
  box-sizing: border-box;
  overflow-x: hidden;
}

.results-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 600;
  color: #111827;
  flex-shrink: 0;

  .t-icon {
    color: #07C05F;
  }
}

.no-results {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 16px;
  color: #6B7280;
  font-family: "PingFang SC";
  font-size: 14px;
  text-align: center;
  background: #F9FAFB;
  border-radius: 8px;
  border: 1px dashed #E7E7E7;
}

.results-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-right: 4px;

  &::-webkit-scrollbar {
    width: 6px;
  }

  &::-webkit-scrollbar-track {
    background: #f5f5f5;
    border-radius: 3px;
  }

  &::-webkit-scrollbar-thumb {
    background: #d0d0d0;
    border-radius: 3px;
    transition: background 0.2s;

    &:hover {
      background: #07C05F;
    }
  }
}

.result-card {
  border: 1px solid #E7E7E7;
  border-radius: 8px;
  background: #fff;
  padding: 14px;
  transition: all 0.2s ease;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
  width: 100%;
  box-sizing: border-box;
  min-width: 0;
  overflow: hidden;

  &:hover {
    border-color: #07C05F;
    box-shadow: 0 2px 8px rgba(7, 192, 95, 0.12);
  }
}

.result-header {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 0;
  border-bottom: none;
  cursor: pointer;
  user-select: none;
  padding: 4px;
  margin: -4px;
  border-radius: 6px;

  &:hover {
    background-color: #F9FAFB;
  }
}

.result-card.expanded .result-header {
  margin-bottom: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid #E7E7E7;
}

.result-question-wrapper {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
}

.result-question {
  flex: 1;
  min-width: 0;
  font-family: "PingFang SC";
  font-size: 14px;
  font-weight: 600;
  color: #111827;
  line-height: 1.6;
  word-break: break-word;
}

.expand-icon {
  flex-shrink: 0;
  font-size: 18px;
  color: #6B7280;
  transition: transform 0.2s ease;
  cursor: pointer;

  &:hover {
    color: #07C05F;
  }
}

.result-meta {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.score-tag,
.match-type-tag {
  font-size: 12px;
  padding: 4px 8px;
  border-radius: 6px;
  font-family: "PingFang SC";
}

.result-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-top: 12px;
  margin-top: 12px;
  border-top: 1px solid #F3F4F6;
}

// Slide down animation
.slide-down-enter-active,
.slide-down-leave-active {
  transition: all 0.3s ease;
  overflow: hidden;
}

.slide-down-enter-from {
  opacity: 0;
  max-height: 0;
  padding-top: 0;
  margin-top: 0;
}

.slide-down-enter-to {
  opacity: 1;
  max-height: 1000px;
}

.slide-down-leave-from {
  opacity: 1;
  max-height: 1000px;
}

.slide-down-leave-to {
  opacity: 0;
  max-height: 0;
  padding-top: 0;
  margin-top: 0;
}

.result-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.section-label {
  font-family: "PingFang SC";
  font-size: 12px;
  font-weight: 600;
  color: #6B7280;
  margin-bottom: 4px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.result-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  width: 100%;
  min-width: 0;
}

:deep(.result-tags .t-tag) {
  max-width: 100%;
  min-width: 0;
  word-break: break-word;
  overflow-wrap: break-word;
}

:deep(.result-tags .t-tag__text) {
  display: inline-block;
  max-width: 100%;
  word-break: break-word;
  overflow-wrap: break-word;
  white-space: normal;
  line-height: 1.4;
}
</style>



