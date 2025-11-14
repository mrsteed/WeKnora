<template>
  <div class="faq-manager">
    <!-- Header -->
    <div class="faq-header">
      <h2>{{ $t('knowledgeEditor.faq.title') }}</h2>
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
        <div v-if="entries.length > 0" class="faq-card-list">
        <div
          v-for="entry in entries"
          :key="entry.id"
          class="faq-card"
          :class="{ 'selected': selectedRowKeys.includes(entry.id) }"
        >
          <!-- Card Header -->
          <div class="faq-card-header">
            <t-checkbox
              :checked="selectedRowKeys.includes(entry.id)"
              @change="(checked: boolean) => handleCardSelect(entry.id, checked)"
              @click.stop
            />
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
                  <div class="card-menu-item" @click="handleMenuEdit(entry)">
                    <t-icon class="menu-icon" name="edit" />
                    <span>{{ $t('common.edit') }}</span>
                  </div>
                  <div class="card-menu-item danger" @click="handleMenuDelete(entry)">
                    <t-icon class="menu-icon" name="delete" />
                    <span>{{ $t('common.delete') }}</span>
                  </div>
                </div>
              </template>
            </t-popup>
          </div>

          <!-- Card Body -->
          <div class="faq-card-body">
            <!-- Answers Section -->
            <div class="faq-section answers">
              <div class="faq-section-label">{{ $t('knowledgeEditor.faq.answers') }}</div>
              <div class="faq-tags">
                <t-tooltip
                  v-for="answer in entry.answers"
                  :key="answer"
                  :content="answer"
                  placement="top"
                >
                  <t-tag
                    size="small"
                    theme="success"
                    variant="light"
                    class="answer-tag"
                  >
                    {{ answer }}
                  </t-tag>
                </t-tooltip>
                <span v-if="!entry.answers?.length" class="empty-tip">
                  {{ $t('knowledgeEditor.faq.noAnswer') }}
                </span>
              </div>
            </div>

            <!-- Similar Questions Section -->
            <div class="faq-section similar">
              <div class="faq-section-label">{{ $t('knowledgeEditor.faq.similarQuestions') }}</div>
              <div class="faq-tags">
                <t-tooltip
                  v-for="question in entry.similar_questions"
                  :key="question"
                  :content="question"
                  placement="top"
                >
                  <t-tag
                    size="small"
                    variant="light-outline"
                    class="question-tag"
                  >
                    {{ question }}
                  </t-tag>
                </t-tooltip>
                <span v-if="!entry.similar_questions?.length" class="empty-tip">
                  {{ $t('knowledgeEditor.faq.noSimilar') }}
                </span>
              </div>
            </div>

            <!-- Negative Questions Section -->
            <div class="faq-section negative">
              <div class="faq-section-label">{{ $t('knowledgeEditor.faq.negativeQuestions') }}</div>
              <div class="faq-tags">
                <t-tooltip
                  v-for="question in entry.negative_questions"
                  :key="question"
                  :content="question"
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
                </t-tooltip>
                <span v-if="!entry.negative_questions?.length" class="empty-tip">
                  {{ $t('knowledgeEditor.faq.noNegative') }}
                </span>
              </div>
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

    <!-- Editor Dialog -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="editorVisible" class="faq-editor-overlay" @click.self="editorVisible = false">
          <div class="faq-editor-modal">
            <!-- 关闭按钮 -->
            <button class="close-btn" @click="editorVisible = false" :aria-label="$t('general.close')">
              <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
                <path d="M15 5L5 15M5 5L15 15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
              </svg>
            </button>

            <div class="faq-editor-container">
              <div class="faq-editor-header">
                <h2 class="editor-title">{{ editorMode === 'create' ? $t('knowledgeEditor.faq.editorCreate') : $t('knowledgeEditor.faq.editorEdit') }}</h2>
              </div>

              <div class="faq-editor-content">
                <t-form
                  ref="editorFormRef"
                  :data="editorForm"
                  :rules="editorRules"
                  layout="vertical"
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

              <div class="faq-editor-footer">
                <t-button theme="default" variant="outline" @click="editorVisible = false">
                  {{ $t('common.cancel') }}
                </t-button>
                <t-button theme="primary" @click="handleSubmitEntry" :loading="savingEntry">
                  {{ editorMode === 'create' ? $t('knowledgeEditor.faq.editorCreate') : $t('common.save') }}
                </t-button>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Import Dialog -->
    <t-dialog
      v-model:visible="importVisible"
      :header="$t('knowledgeEditor.faqImport.title')"
      :confirm-btn="{
        content: $t('knowledgeEditor.faqImport.importButton'),
        loading: importState.importing
      }"
      :cancel-btn="$t('common.cancel')"
      @confirm="handleImport"
      width="600px"
      class="faq-import-dialog"
    >
      <div class="import-dialog-content">
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
    </t-dialog>

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
              :loading="searching"
              @click="handleSearch"
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
import { ref, reactive, watch, onMounted, computed } from 'vue'
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
      showMore: false
    }))
    
    if (append) {
      entries.value = [...entries.value, ...newEntries]
    } else {
      entries.value = newEntries
    }

    // 判断是否还有更多数据
    hasMore.value = entries.value.length < (pageData.total || 0)
    currentPage++
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

onMounted(() => {
  if (props.kbId) {
    loadEntries()
  }
})
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

  h2 {
    margin: 0;
    color: #000000e6;
    font-family: "PingFang SC";
    font-size: 24px;
    font-weight: 600;
    line-height: 32px;
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

// 卡片列表样式
.faq-card-list {
  display: grid;
  gap: 10px;
  grid-template-columns: 1fr;
  width: 100%;
  min-width: 0;
}

.faq-card {
  border: 1px solid #f0f0f0;
  border-radius: 4px;
  background: #fff;
  padding: 10px 12px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.03);
  transition: all 0.2s ease;
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;

  &:hover {
    border-color: #07c05f;
    box-shadow: 0 2px 8px rgba(7, 192, 95, 0.1);
  }

  &.selected {
    border-color: #07c05f;
    background: #f0fdf4;
  }
}

.faq-card-header {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid #f5f5f5;
  position: relative;

  :deep(.t-checkbox) {
    flex-shrink: 0;
    margin-top: 1px;
  }
}

.card-more-btn {
  display: flex;
  width: 20px;
  height: 20px;
  justify-content: center;
  align-items: center;
  border-radius: 3px;
  cursor: pointer;
  transition: all 0.2s ease;
  flex-shrink: 0;
  margin-left: auto;

  &:hover {
    background: #f5f5f5;
  }

  .more-icon {
    width: 14px;
    height: 14px;
  }
}

.card-menu {
  display: flex;
  flex-direction: column;
  min-width: 120px;
}

.card-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  color: #000000e6;
  font-family: "PingFang SC";
  font-size: 13px;
  transition: all 0.2s ease;

  &:hover {
    background: #f5f5f5;
  }

  &.danger {
    color: #fa5151;
  }

  .menu-icon {
    font-size: 14px;
    flex-shrink: 0;
  }
}

.faq-question {
  flex: 1;
  color: #000000e6;
  font-family: "PingFang SC";
  font-size: 13px;
  font-weight: 600;
  line-height: 18px;
  word-break: break-word;
  min-width: 0;
  overflow: hidden;
}

.faq-card-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.faq-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  overflow: hidden;

  .faq-section-label {
    color: #00000099;
    font-family: "PingFang SC";
    font-size: 11px;
    font-weight: 500;
  }
}

.faq-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  min-height: 20px;
  min-width: 0;
}

.answer-tag,
.question-tag {
  font-size: 11px;
  padding: 2px 6px;
  max-width: 100%;
  min-width: 0;
  
  :deep(.t-tag__text) {
    display: inline-block;
    vertical-align: middle;
    max-width: 200px;
    word-break: break-word;
    white-space: normal;
    overflow-wrap: break-word;
  }
}

.answer-tag {
  max-width: 100%;
  
  :deep(.t-tag) {
    max-width: 100%;
    min-width: 0;
    display: inline-block;
    vertical-align: top;
  }
  
  :deep(.t-tag__text) {
    max-width: 100%;
    white-space: normal;
    word-break: break-word;
    overflow-wrap: break-word;
    line-height: 1.4;
    display: block;
  }
}

// 确保 tag 本身不会超出容器
:deep(.t-tag) {
  max-width: 100%;
  min-width: 0;
}

.empty-tip {
  color: #999;
  font-size: 11px;
  font-style: italic;
}


.faq-load-more,
.faq-no-more {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 16px;
  color: #999;
  font-size: 12px;
  font-family: "PingFang SC";
}

.faq-no-more {
  color: #ccc;
}

// 空状态样式
.faq-empty-state {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 400px;
  padding: 40px 20px;

  .empty-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    text-align: center;
  }

  .empty-icon {
    color: #d9d9d9;
  }

  .empty-text {
    color: #00000099;
    font-family: "PingFang SC";
    font-size: 16px;
    font-weight: 600;
    line-height: 24px;
  }

  .empty-desc {
    color: #00000066;
    font-family: "PingFang SC";
    font-size: 14px;
    font-weight: 400;
    line-height: 22px;
  }
}

// 导入对话框样式
:deep(.faq-import-dialog) {
  // 确保对话框容器垂直居中
  &.t-dialog__ctx {
    display: flex;
    align-items: center;
    justify-content: center;
  }

  // 覆盖其他文件中的全局样式，确保垂直居中
  .t-dialog__position {
    display: flex !important;
    align-items: center !important;
    justify-content: center !important;
    padding: 20px !important;
    min-height: 100%;
    top: 0 !important;
    margin: 0 !important;
  }

  // 特别处理 t-dialog--top 类的情况，覆盖全局样式
  .t-dialog__position.t-dialog--top {
    padding-top: 0 !important;
    display: flex !important;
    align-items: center !important;
    justify-content: center !important;
  }

  .t-dialog {
    margin: 0 !important;
    max-height: 90vh;
    max-width: 90vw;
    display: flex;
    flex-direction: column;
    width: 600px;
  }

  // 覆盖 t-dialog--default 类的边距
  .t-dialog--default {
    margin: 0 !important;
  }

  .t-dialog__header {
    flex-shrink: 0;
    padding: 20px 24px;
    border-bottom: 1px solid #e5e7eb;
  }

  .t-dialog__body {
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

  .t-dialog__footer {
    flex-shrink: 0;
    border-top: 1px solid #e5e7eb;
  }
}

// 导入对话框内容区域
.import-dialog-content {
  padding: 0;
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

// 响应式布局
@media (min-width: 900px) {
  .faq-card-list {
    grid-template-columns: repeat(4, 1fr);
    gap: 8px;
  }
}

@media (min-width: 1250px) {
  .faq-card-list {
    grid-template-columns: repeat(5, 1fr);
  }
}

@media (min-width: 1600px) {
  .faq-card-list {
    grid-template-columns: repeat(6, 1fr);
  }
}

@media (min-width: 2000px) {
  .faq-card-list {
    grid-template-columns: repeat(7, 1fr);
  }
}

@media (min-width: 2400px) {
  .faq-card-list {
    grid-template-columns: repeat(8, 1fr);
  }
}

// 卡片菜单弹窗样式
:deep(.faq-card-popup) {
  z-index: 99 !important;

  .t-popup__content {
    padding: 4px 0 !important;
    margin-top: 4px !important;
    min-width: 120px;
  }
}

// FAQ 编辑器对话框样式 - 参考设置页面风格
.faq-editor-overlay {
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

.faq-editor-modal {
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
}

.close-btn {
  position: absolute;
  top: 16px;
  right: 16px;
  width: 32px;
  height: 32px;
  border: none;
  background: transparent;
  color: #666666;
  cursor: pointer;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
  z-index: 10;

  &:hover {
    background: #f5f5f5;
    color: #333333;
  }
}

.faq-editor-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.faq-editor-header {
  padding: 24px 24px 16px;
  border-bottom: 1px solid #e5e7eb;

  .editor-title {
    margin: 0;
    font-family: "PingFang SC";
    font-size: 18px;
    font-weight: 600;
    color: #000000e6;
  }
}

.faq-editor-content {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
}

.faq-editor-footer {
  padding: 16px 24px;
  border-top: 1px solid #e5e7eb;
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  flex-shrink: 0;
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
    width: 20px;
    height: 20px;
    min-width: 20px;
    padding: 0;
    font-family: "PingFang SC";
    transition: all 0.2s ease;
  }

  :deep(.add-item-btn) {
    background: transparent !important;
    border: 1px solid #07c05f !important;
    border-radius: 3px !important;
    color: #07c05f !important;

    &:hover:not(:disabled) {
      background: #07c05f1a !important;
      border-color: #07c05f !important;
    }

    &:active:not(:disabled) {
      background: #07c05f33 !important;
    }

    &:disabled {
      opacity: 0.5;
      cursor: not-allowed;
    }
  }
}

.item-count {
  font-size: 13px;
  color: #00000099;
  font-family: "PingFang SC";
  font-weight: 500;
  margin-bottom: 12px;
  text-align: right;
}

.item-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 8px;
}

.section-divider {
  height: 1px;
  background: #e5e7eb;
  margin: 16px 0;
}

.item-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  transition: all 0.2s ease;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);

  &.answer-row {
    align-items: flex-start;
  }

  &:hover {
    background: #fafafa;
    border-color: #07c05f;
    box-shadow: 0 2px 6px rgba(7, 192, 95, 0.1);
  }

  &.negative {
    background: #fffbf0;
    border-color: #ffe7ba;

    &:hover {
      background: #fff7e6;
      border-color: #ffd591;
      box-shadow: 0 2px 6px rgba(255, 193, 7, 0.12);
    }
  }

  .item-index {
    flex-shrink: 0;
    width: 20px;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(135deg, #07c05f 0%, #05a04f 100%);
    color: #fff;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    font-family: "PingFang SC";
    box-shadow: 0 1px 3px rgba(7, 192, 95, 0.2);
  }

  .item-content {
    flex: 1;
    font-size: 13px;
    line-height: 1.5;
    color: #000000e6;
    font-family: "PingFang SC";
    white-space: pre-wrap;
    word-break: break-word;
    padding: 0;
  }

  .remove-item-btn {
    flex-shrink: 0;
    color: #8c8c8c;
    padding: 0;
    width: 20px;
    height: 20px;
    min-width: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 4px;
    transition: all 0.2s ease;
    opacity: 0.6;

    &:hover {
      color: #fa5151;
      background: #fff1f0;
      opacity: 1;
    }

    :deep(.t-icon) {
      font-size: 12px;
    }
  }

  &.answer-row .remove-item-btn {
    margin-top: 2px;
  }
}

.form-tip {
  margin-top: 6px;
  font-size: 12px;
  color: #00000066;
  font-family: "PingFang SC";
}

// 表单样式优化
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
    color: #000000e6;
    margin-bottom: 8px;
    line-height: 22px;
  }

  .t-form-item__content {
    margin: 0;
  }
}

:deep(.t-input),
:deep(.t-textarea) {
  font-family: "PingFang SC";
  font-size: 14px;
  border-radius: 6px;
  transition: all 0.2s ease;

  &:focus {
    border-color: #07c05f;
    box-shadow: 0 0 0 2px rgba(7, 192, 95, 0.1);
  }
}

:deep(.t-textarea__inner) {
  font-family: "PingFang SC";
  font-size: 14px;
  line-height: 1.6;
  resize: vertical;
}

:deep(.t-button--theme-primary) {
  background-color: #07c05f;
  border-color: #07c05f;
  
  &:hover {
    background-color: #05a04f;
    border-color: #05a04f;
  }
}

// 弹窗动画
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}

.modal-enter-active .faq-editor-modal,
.modal-leave-active .faq-editor-modal {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}

.modal-enter-from .faq-editor-modal,
.modal-leave-to .faq-editor-modal {
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

// Search test drawer styles
:deep(.faq-search-drawer) {
  .t-drawer__body {
    padding: 20px;
    overflow-y: auto;
  }
}

.search-test-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
  height: 100%;
}

.search-form {
  flex-shrink: 0;
}

.form-item-compact {
  margin-bottom: 16px;

  &:last-child {
    margin-bottom: 0;
  }
}

:deep(.form-item-compact .t-form-item__label) {
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 500;
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
}

:deep(.slider-wrapper .t-slider) {
  flex: 1;
  min-width: 0;
}

.slider-value {
  flex-shrink: 0;
  min-width: 40px;
  text-align: right;
  font-family: "PingFang SC";
  font-size: 13px;
  font-weight: 500;
  color: #000000e6;
}

.form-tip {
  margin-top: 4px;
  font-size: 11px;
  color: #00000066;
  font-family: "PingFang SC";
  line-height: 16px;
}

.search-results {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-top: 1px solid #e5e7eb;
  padding-top: 16px;
  width: 100%;
  box-sizing: border-box;
  overflow-x: hidden;
}

.results-header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
  font-family: "PingFang SC";
  font-size: 13px;
  font-weight: 500;
  color: #000000e6;
  flex-shrink: 0;
}

.no-results {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px 16px;
  color: #00000066;
  font-family: "PingFang SC";
  font-size: 13px;
  text-align: center;
}

.results-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
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
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  background: #fff;
  padding: 10px;
  transition: all 0.2s ease;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.03);
  width: 100%;
  box-sizing: border-box;
  min-width: 0;
  overflow: hidden;

  &:hover {
    border-color: #07c05f;
    box-shadow: 0 2px 6px rgba(7, 192, 95, 0.1);
  }
}

.result-header {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 0;
  border-bottom: none;
  cursor: pointer;
  user-select: none;
  transition: background-color 0.2s ease;

  &:hover {
    background-color: #fafafa;
    margin: -10px;
    padding: 10px;
    border-radius: 4px;
  }
}

.result-card.expanded .result-header {
  margin-bottom: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid #f5f5f5;
}

.result-question-wrapper {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
}

.result-question {
  flex: 1;
  min-width: 0;
  font-family: "PingFang SC";
  font-size: 13px;
  font-weight: 600;
  color: #000000e6;
  line-height: 18px;
  word-break: break-word;
}

.expand-icon {
  flex-shrink: 0;
  font-size: 16px;
  color: #00000066;
  transition: transform 0.2s ease;
  cursor: pointer;
}

.result-meta {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.score-tag,
.match-type-tag {
  font-size: 10px;
  padding: 2px 5px;
}

.result-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 10px;
  margin-top: 10px;
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
  gap: 4px;
}

.section-label {
  font-family: "PingFang SC";
  font-size: 10px;
  font-weight: 500;
  color: #00000099;
  margin-bottom: 2px;
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


