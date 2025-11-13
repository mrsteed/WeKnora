<template>
  <div class="model-settings">
    <div class="section-header">
      <h2>{{ $t('modelSettings.title') }}</h2>
      <p class="section-description">{{ $t('modelSettings.description') }}</p>
    </div>

    <!-- 对话模型 -->
    <div class="model-category-section" data-model-type="chat">
      <div class="category-header">
        <div class="header-info">
          <h3>{{ $t('modelSettings.chat.title') }}</h3>
          <p>{{ $t('modelSettings.chat.desc') }}</p>
        </div>
        <t-button size="small" theme="primary" @click="openAddDialog('chat')">
          <template #icon>
            <t-icon name="add" />
          </template>
          {{ $t('modelSettings.actions.addModel') }}
        </t-button>
      </div>
      
      <div v-if="chatModels.length > 0" class="model-list-container">
        <div v-for="model in chatModels" :key="model.id" class="model-card">
          <div class="model-info">
            <div class="model-name">
              {{ model.name }}
              <t-tag v-if="model.isDefault" theme="success" size="small">{{ $t('common.default') }}</t-tag>
            </div>
            <div class="model-meta">
              <span class="source-tag">{{ model.source === 'local' ? 'Ollama' : $t('modelSettings.source.remote') }}</span>
              <!-- <span class="model-id">{{ model.modelName }}</span> -->
            </div>
          </div>
          <div class="model-actions">
            <t-dropdown 
              :options="getModelOptions('chat', model)" 
              @click="(data: any) => handleMenuAction(data, 'chat', model)"
              placement="bottom-right"
              attach="body"
            >
              <t-button variant="text" shape="square" size="small" class="more-btn">
                <t-icon name="more" />
              </t-button>
            </t-dropdown>
          </div>
        </div>
      </div>
      <div v-else class="empty-state">
        <p class="empty-text">{{ $t('modelSettings.chat.empty') }}</p>
        <t-button theme="default" variant="outline" size="small" @click="openAddDialog('chat')">
          {{ $t('modelSettings.actions.addModel') }}
        </t-button>
      </div>
    </div>

    <!-- Embedding 模型 -->
    <div class="model-category-section" data-model-type="embedding">
      <div class="category-header">
        <div class="header-info">
          <h3>{{ $t('modelSettings.embedding.title') }}</h3>
          <p>{{ $t('modelSettings.embedding.desc') }}</p>
        </div>
        <t-button size="small" theme="primary" @click="openAddDialog('embedding')">
          <template #icon>
            <t-icon name="add" />
          </template>
          {{ $t('modelSettings.actions.addModel') }}
        </t-button>
      </div>
      
      <div v-if="embeddingModels.length > 0" class="model-list-container">
        <div v-for="model in embeddingModels" :key="model.id" class="model-card">
          <div class="model-info">
            <div class="model-name">
              {{ model.name }}
              <t-tag v-if="model.isDefault" theme="success" size="small">{{ $t('common.default') }}</t-tag>
            </div>
            <div class="model-meta">
              <span class="source-tag">{{ model.source === 'local' ? 'Ollama' : $t('modelSettings.source.remote') }}</span>
              <!-- <span class="model-id">{{ model.modelName }}</span> -->
              <span v-if="model.dimension" class="dimension">{{ $t('model.editor.dimensionLabel') }}: {{ model.dimension }}</span>
            </div>
          </div>
          <div class="model-actions">
            <t-dropdown 
              :options="getModelOptions('embedding', model)" 
              @click="(data: any) => handleMenuAction(data, 'embedding', model)"
              placement="bottom-right"
              attach="body"
            >
              <t-button variant="text" shape="square" size="small" class="more-btn">
                <t-icon name="more" />
              </t-button>
            </t-dropdown>
          </div>
        </div>
      </div>
      <div v-else class="empty-state">
        <p class="empty-text">{{ $t('modelSettings.embedding.empty') }}</p>
        <t-button theme="default" variant="outline" size="small" @click="openAddDialog('embedding')">
          {{ $t('modelSettings.actions.addModel') }}
        </t-button>
      </div>
    </div>

    <!-- ReRank 模型 -->
    <div class="model-category-section" data-model-type="rerank">
      <div class="category-header">
        <div class="header-info">
          <h3>{{ $t('modelSettings.rerank.title') }}</h3>
          <p>{{ $t('modelSettings.rerank.desc') }}</p>
        </div>
        <t-button size="small" theme="primary" @click="openAddDialog('rerank')">
          <template #icon>
            <t-icon name="add" />
          </template>
          {{ $t('modelSettings.actions.addModel') }}
        </t-button>
      </div>
      
      <div v-if="rerankModels.length > 0" class="model-list-container">
        <div v-for="model in rerankModels" :key="model.id" class="model-card">
          <div class="model-info">
            <div class="model-name">
              {{ model.name }}
              <t-tag v-if="model.isDefault" theme="success" size="small">{{ $t('common.default') }}</t-tag>
            </div>
            <div class="model-meta">
              <span class="source-tag">{{ model.source === 'local' ? 'Ollama' : $t('modelSettings.source.remote') }}</span>
              <!-- <span class="model-id">{{ model.modelName }}</span> -->
            </div>
          </div>
          <div class="model-actions">
            <t-dropdown 
              :options="getModelOptions('rerank', model)" 
              @click="(data: any) => handleMenuAction(data, 'rerank', model)"
              placement="bottom-right"
              attach="body"
            >
              <t-button variant="text" shape="square" size="small" class="more-btn">
                <t-icon name="more" />
              </t-button>
            </t-dropdown>
          </div>
        </div>
      </div>
      <div v-else class="empty-state">
        <p class="empty-text">{{ $t('modelSettings.rerank.empty') }}</p>
        <t-button theme="default" variant="outline" size="small" @click="openAddDialog('rerank')">
          {{ $t('modelSettings.actions.addModel') }}
        </t-button>
      </div>
    </div>

    <!-- VLLM 视觉模型 -->
    <div class="model-category-section" data-model-type="vllm">
      <div class="category-header">
        <div class="header-info">
          <h3>{{ $t('modelSettings.vllm.title') }}</h3>
          <p>{{ $t('modelSettings.vllm.desc') }}</p>
        </div>
        <t-button size="small" theme="primary" @click="openAddDialog('vllm')">
          <template #icon>
            <t-icon name="add" />
          </template>
          {{ $t('modelSettings.actions.addModel') }}
        </t-button>
      </div>
      
      <div v-if="vllmModels.length > 0" class="model-list-container">
        <div v-for="model in vllmModels" :key="model.id" class="model-card">
          <div class="model-info">
            <div class="model-name">
              {{ model.name }}
              <t-tag v-if="model.isDefault" theme="success" size="small">{{ $t('common.default') }}</t-tag>
            </div>
            <div class="model-meta">
              <span class="source-tag">{{ model.source === 'local' ? 'Ollama' : $t('modelSettings.source.openaiCompatible') }}</span>
              <!-- <span class="model-id">{{ model.modelName }}</span> -->
            </div>
          </div>
          <div class="model-actions">
            <t-dropdown 
              :options="getModelOptions('vllm', model)" 
              @click="(data: any) => handleMenuAction(data, 'vllm', model)"
              placement="bottom-right"
              attach="body"
            >
              <t-button variant="text" shape="square" size="small" class="more-btn">
                <t-icon name="more" />
              </t-button>
            </t-dropdown>
          </div>
        </div>
      </div>
      <div v-else class="empty-state">
        <p class="empty-text">暂无 VLLM 视觉模型</p>
        <t-button theme="default" variant="outline" size="small" @click="openAddDialog('vllm')">
          添加模型
        </t-button>
      </div>
    </div>

    <!-- 模型编辑器弹窗 -->
    <ModelEditorDialog
      v-model:visible="showDialog"
      :model-type="currentModelType"
      :model-data="editingModel"
      @confirm="handleModelSave"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import ModelEditorDialog from '@/components/ModelEditorDialog.vue'
import { listModels, createModel, updateModel as updateModelAPI, deleteModel as deleteModelAPI, type ModelConfig } from '@/api/model'

const { t } = useI18n()

const showDialog = ref(false)
const currentModelType = ref<'chat' | 'embedding' | 'rerank' | 'vllm'>('chat')
const editingModel = ref<any>(null)
const loading = ref(true)

// 模型列表数据
const allModels = ref<ModelConfig[]>([])

// 根据类型过滤并去重模型
const chatModels = computed(() => 
  deduplicateModels(
    allModels.value
      .filter(m => m.type === 'KnowledgeQA')
      .map(convertToLegacyFormat)
  )
)

const embeddingModels = computed(() => 
  deduplicateModels(
    allModels.value
      .filter(m => m.type === 'Embedding')
      .map(convertToLegacyFormat)
  )
)

const rerankModels = computed(() => 
  deduplicateModels(
    allModels.value
      .filter(m => m.type === 'Rerank')
      .map(convertToLegacyFormat)
  )
)

const vllmModels = computed(() => 
  deduplicateModels(
    allModels.value
      .filter(m => m.type === 'VLLM')
      .map(convertToLegacyFormat)
  )
)

// 将后端模型格式转换为旧的前端格式
function convertToLegacyFormat(model: ModelConfig) {
  return {
    id: model.id!,
    name: model.name,
    source: model.source,
    modelName: model.name,  // 显示名称作为模型名
    baseUrl: model.parameters.base_url || '',
    apiKey: model.parameters.api_key || '',
    dimension: model.parameters.embedding_parameters?.dimension,
    isDefault: model.is_default || false
  }
}

// 去重函数：比较除id外的所有字段，相同的只保留第一个
function deduplicateModels(models: any[]) {
  const seen = new Map<string, any>()
  
  return models.filter(model => {
    // 创建一个不包含id的签名用于比较
    const signature = JSON.stringify({
      name: model.name,
      source: model.source,
      modelName: model.modelName,
      baseUrl: model.baseUrl,
      apiKey: model.apiKey,
      dimension: model.dimension
    })
    
    if (seen.has(signature)) {
      // 如果已经存在相同的模型，优先保留默认模型
      const existing = seen.get(signature)
      if (model.isDefault && !existing.isDefault) {
        seen.set(signature, model)
        return true
      }
      return false
    }
    
    seen.set(signature, model)
    return true
  })
}

// 加载模型列表
const loadModels = async () => {
  loading.value = true
  try {
    // 直接获取所有模型，不分类型
    const models = await listModels()
    allModels.value = models
  } catch (error: any) {
    console.error('加载模型列表失败:', error)
    MessagePlugin.error(error.message || '加载模型列表失败')
  } finally {
    loading.value = false
  }
}

// 打开添加对话框
const openAddDialog = (type: 'chat' | 'embedding' | 'rerank' | 'vllm') => {
  currentModelType.value = type
  editingModel.value = null
  showDialog.value = true
}

// 编辑模型
const editModel = (type: 'chat' | 'embedding' | 'rerank' | 'vllm', model: any) => {
  currentModelType.value = type
  editingModel.value = { ...model }
  showDialog.value = true
}

// 保存模型
const handleModelSave = async (modelData: any) => {
  try {
    // 字段校验
    if (!modelData.modelName || !modelData.modelName.trim()) {
      MessagePlugin.warning(t('modelSettings.toasts.nameRequired'))
      return
    }
    
    if (modelData.modelName.trim().length > 100) {
      MessagePlugin.warning(t('modelSettings.toasts.nameTooLong'))
      return
    }
    
    // Remote 类型必须填写 baseUrl
    if (modelData.source === 'remote') {
      if (!modelData.baseUrl || !modelData.baseUrl.trim()) {
        MessagePlugin.warning(t('modelSettings.toasts.baseUrlRequired'))
        return
      }
      
      // 校验 Base URL 格式
      try {
        new URL(modelData.baseUrl.trim())
      } catch {
        MessagePlugin.warning(t('modelSettings.toasts.baseUrlInvalid'))
        return
      }
    }
    
    // Embedding 模型必须填写维度
    if (currentModelType.value === 'embedding') {
      if (!modelData.dimension || modelData.dimension < 128 || modelData.dimension > 4096) {
        MessagePlugin.warning(t('modelSettings.toasts.dimensionInvalid'))
        return
      }
    }
    
    // 将前端格式转换为后端格式
    const apiModelData: ModelConfig = {
      name: modelData.modelName.trim(), // 使用 modelName 作为 name，并去除首尾空格
      type: getModelType(currentModelType.value),
      source: modelData.source,
      description: '',
      parameters: {
        base_url: modelData.baseUrl?.trim() || '',
        api_key: modelData.apiKey?.trim() || '',
        ...(currentModelType.value === 'embedding' && modelData.dimension ? {
          embedding_parameters: {
            dimension: modelData.dimension,
            truncate_prompt_tokens: 0
          }
        } : {})
      },
      is_default: modelData.isDefault || false
    }

    if (editingModel.value && editingModel.value.id) {
      // 更新现有模型
      await updateModelAPI(editingModel.value.id, apiModelData)
      MessagePlugin.success(t('modelSettings.toasts.updated'))
    } else {
      // 添加新模型
      await createModel(apiModelData)
      MessagePlugin.success(t('modelSettings.toasts.added'))
    }
    
    // 重新加载模型列表
    await loadModels()
  } catch (error: any) {
    console.error('保存模型失败:', error)
    MessagePlugin.error(error.message || t('modelSettings.toasts.saveFailed'))
  }
}

// 删除模型
const deleteModel = async (type: 'chat' | 'embedding' | 'rerank' | 'vllm', modelId: string) => {
  try {
    await deleteModelAPI(modelId)
    MessagePlugin.success(t('modelSettings.toasts.deleted'))
    // 重新加载模型列表
    await loadModels()
  } catch (error: any) {
    console.error('删除模型失败:', error)
    MessagePlugin.error(error.message || t('modelSettings.toasts.deleteFailed'))
  }
}

// 设为默认
const setDefault = async (type: 'chat' | 'embedding' | 'rerank' | 'vllm', modelId: string) => {
  try {
    // 更新模型的 is_default 字段
    await updateModelAPI(modelId, { is_default: true })
    MessagePlugin.success(t('modelSettings.toasts.setDefault'))
    // 重新加载模型列表
    await loadModels()
  } catch (error: any) {
    console.error('设置默认模型失败:', error)
    MessagePlugin.error(error.message || t('modelSettings.toasts.setDefaultFailed'))
  }
}

// 获取模型操作菜单选项
const getModelOptions = (type: 'chat' | 'embedding' | 'rerank' | 'vllm', model: any) => {
  const options: any[] = []
  
  // 如果不是默认模型，显示"设为默认"选项
  if (!model.isDefault) {
    options.push({
      content: t('modelSettings.actions.setDefault'),
      value: `set-default-${type}-${model.id}`
    })
  }
  
  // 编辑选项
  options.push({
    content: t('common.edit'),
    value: `edit-${type}-${model.id}`
  })
  
  // 删除选项
  options.push({
    content: t('common.delete'),
    value: `delete-${type}-${model.id}`,
    theme: 'error'
  })
  
  return options
}

// 处理菜单操作
const handleMenuAction = (data: { value: string }, type: 'chat' | 'embedding' | 'rerank' | 'vllm', model: any) => {
  const value = data.value
  
  if (value.indexOf('set-default-') === 0) {
    setDefault(type, model.id)
  } else if (value.indexOf('edit-') === 0) {
    editModel(type, model)
  } else if (value.indexOf('delete-') === 0) {
    // 使用确认对话框进行确认
    if (confirm(t('modelSettings.confirmDelete'))) {
      deleteModel(type, model.id)
    }
  }
}

// 获取后端模型类型
function getModelType(type: 'chat' | 'embedding' | 'rerank' | 'vllm'): 'KnowledgeQA' | 'Embedding' | 'Rerank' | 'VLLM' {
  const typeMap = {
    chat: 'KnowledgeQA' as const,
    embedding: 'Embedding' as const,
    rerank: 'Rerank' as const,
    vllm: 'VLLM' as const
  }
  return typeMap[type]
}

// 组件挂载时加载模型列表
onMounted(() => {
  loadModels()
})
</script>

<style lang="less" scoped>
.model-settings {
  width: 100%;
}

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

.model-category-section {
  margin-bottom: 24px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 24px;
  background: #ffffff;

  &:last-child {
    margin-bottom: 0;
  }
}

.category-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 20px;

  .header-info {
    flex: 1;

    h3 {
      font-size: 16px;
      font-weight: 600;
      color: #333333;
      margin: 0 0 4px 0;
    }

    p {
      font-size: 14px;
      color: #666666;
      margin: 0;
      line-height: 1.5;
    }
  }

}

.model-list-container {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;

  @media (max-width: 768px) {
    grid-template-columns: 1fr;
  }
}

.model-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #fafafa;
  transition: all 0.2s;
  position: relative;
  overflow: visible;

  &:hover {
    border-color: #07C05F;
    background: #ffffff;
  }
}

.model-info {
  flex: 1;
  min-width: 0;

  .model-name {
    font-size: 14px;
    font-weight: 500;
    color: #333333;
    margin-bottom: 4px;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .model-meta {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 12px;
    color: #666666;

    .source-tag {
      padding: 1px 6px;
      background: #e5e7eb;
      border-radius: 3px;
      font-size: 11px;
    }

    .model-id {
      font-family: monospace;
      color: #666666;
    }

    .dimension {
      color: #999999;
    }
  }
}

.model-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  opacity: 0;
  transition: opacity 0.2s ease;
  position: relative;
  z-index: 1001; // 确保高于设置窗口的z-index (9999)

  .more-btn {
    color: #666666;
    padding: 4px;
    
    &:hover {
      background: #f5f7fa;
      color: #333333;
    }
  }
}

.model-card:hover .model-actions {
  opacity: 1;
}

.empty-state {
  padding: 80px 0;
  text-align: center;

  .empty-text {
    font-size: 14px;
    color: #999999;
    margin: 0 0 16px 0;
  }
}
</style>
