<template>
  <div class="kb-advanced-settings">
    <div class="section-header">
      <h2>{{ $t('knowledgeEditor.advanced.title') }}</h2>
      <p class="section-description">{{ $t('knowledgeEditor.advanced.description') }}</p>
    </div>

    <div class="settings-group">
      <!-- Multimodal feature -->
      <div class="setting-row">
        <div class="setting-info">
          <label>{{ $t('knowledgeEditor.advanced.multimodal.label') }}</label>
          <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.description') }}</p>
        </div>
        <div class="setting-control">
          <t-switch
            v-model="localMultimodal.enabled"
            @change="handleMultimodalToggle"
            size="large"
          />
        </div>
      </div>

      <!-- Multimodal storage configuration -->
      <div v-if="localMultimodal.enabled" class="subsection">
        <!-- VLLM model -->
        <div class="setting-row">
          <div class="setting-info">
            <label>{{ $t('knowledgeEditor.advanced.multimodal.vllmLabel') }} <span class="required">*</span></label>
            <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.vllmDescription') }}</p>
          </div>
          <div class="setting-control">
            <ModelSelector
              ref="vllmSelectorRef"
              model-type="VLLM"
              :selected-model-id="localMultimodal.vllmModelId"
              :all-models="allModels"
              @update:selected-model-id="handleVLLMChange"
              @add-model="handleAddModel('vllm')"
              :placeholder="$t('knowledgeEditor.advanced.multimodal.vllmPlaceholder')"
            />
          </div>
        </div>

        <div class="subsection-header">
          <h4>{{ $t('knowledgeEditor.advanced.multimodal.storageTitle') }} <span class="required">*</span></h4>
        </div>
        
        <div class="setting-row">
          <div class="setting-info">
            <label>{{ $t('knowledgeEditor.advanced.multimodal.storageTypeLabel') }} <span class="required">*</span></label>
            <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.storageTypeDescription') }}</p>
            <!-- Warning message when MinIO is not enabled -->
            <t-alert
              v-if="!isMinioEnabled"
              theme="warning"
              :message="$t('knowledgeEditor.advanced.multimodal.minioDisabledWarning')"
              style="margin-top: 8px;"
            />
          </div>
          <div class="setting-control">
            <t-radio-group v-model="localMultimodal.storageType" @change="handleStorageTypeChange">
              <t-radio value="minio" :disabled="!isMinioEnabled">
                {{ $t('knowledgeEditor.advanced.multimodal.storageTypeOptions.minio') }}
              </t-radio>
              <t-radio value="cos">{{ $t('knowledgeEditor.advanced.multimodal.storageTypeOptions.cos') }}</t-radio>
            </t-radio-group>
          </div>
        </div>

        <!-- MinIO configuration -->
        <div v-if="localMultimodal.storageType === 'minio'" class="storage-config">
          <div class="setting-row">
            <div class="setting-info">
              <label>{{ $t('knowledgeEditor.advanced.multimodal.minio.bucketLabel') }} <span class="required">*</span></label>
              <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.minio.bucketDescription') }}</p>
            </div>
            <div class="setting-control">
              <t-input
                v-model="localMultimodal.minio.bucketName"
                :placeholder="$t('knowledgeEditor.advanced.multimodal.minio.bucketPlaceholder')"
                @change="handleConfigChange"
                style="width: 280px;"
              />
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label>{{ $t('knowledgeEditor.advanced.multimodal.minio.useSslLabel') }}</label>
              <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.minio.useSslDescription') }}</p>
            </div>
            <div class="setting-control">
              <t-switch
                v-model="localMultimodal.minio.useSSL"
                @change="handleConfigChange"
                size="large"
              />
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label>{{ $t('knowledgeEditor.advanced.multimodal.minio.pathPrefixLabel') }}</label>
              <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.minio.pathPrefixDescription') }}</p>
            </div>
            <div class="setting-control">
              <t-input
                v-model="localMultimodal.minio.pathPrefix"
                :placeholder="$t('knowledgeEditor.advanced.multimodal.minio.pathPrefixPlaceholder')"
                @change="handleConfigChange"
                style="width: 280px;"
              />
            </div>
          </div>
        </div>

        <!-- COS configuration -->
        <div v-if="localMultimodal.storageType === 'cos'" class="storage-config">
          <div class="setting-row">
            <div class="setting-info">
              <label>{{ $t('knowledgeEditor.advanced.multimodal.cos.secretIdLabel') }} <span class="required">*</span></label>
              <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.cos.secretIdDescription') }}</p>
            </div>
            <div class="setting-control">
              <t-input
                v-model="localMultimodal.cos.secretId"
                :placeholder="$t('knowledgeEditor.advanced.multimodal.cos.secretIdPlaceholder')"
                @change="handleConfigChange"
                style="width: 280px;"
              />
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label>{{ $t('knowledgeEditor.advanced.multimodal.cos.secretKeyLabel') }} <span class="required">*</span></label>
              <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.cos.secretKeyDescription') }}</p>
            </div>
            <div class="setting-control">
              <t-input
                v-model="localMultimodal.cos.secretKey"
                type="password"
                :placeholder="$t('knowledgeEditor.advanced.multimodal.cos.secretKeyPlaceholder')"
                @change="handleConfigChange"
                style="width: 280px;"
              />
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label>{{ $t('knowledgeEditor.advanced.multimodal.cos.regionLabel') }} <span class="required">*</span></label>
              <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.cos.regionDescription') }}</p>
            </div>
            <div class="setting-control">
              <t-input
                v-model="localMultimodal.cos.region"
                :placeholder="$t('knowledgeEditor.advanced.multimodal.cos.regionPlaceholder')"
                @change="handleConfigChange"
                style="width: 280px;"
              />
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label>{{ $t('knowledgeEditor.advanced.multimodal.cos.bucketLabel') }} <span class="required">*</span></label>
              <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.cos.bucketDescription') }}</p>
            </div>
            <div class="setting-control">
              <t-input
                v-model="localMultimodal.cos.bucketName"
                :placeholder="$t('knowledgeEditor.advanced.multimodal.cos.bucketPlaceholder')"
                @change="handleConfigChange"
                style="width: 280px;"
              />
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label>{{ $t('knowledgeEditor.advanced.multimodal.cos.appIdLabel') }} <span class="required">*</span></label>
              <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.cos.appIdDescription') }}</p>
            </div>
            <div class="setting-control">
              <t-input
                v-model="localMultimodal.cos.appId"
                :placeholder="$t('knowledgeEditor.advanced.multimodal.cos.appIdPlaceholder')"
                @change="handleConfigChange"
                style="width: 280px;"
              />
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label>{{ $t('knowledgeEditor.advanced.multimodal.cos.pathPrefixLabel') }}</label>
              <p class="desc">{{ $t('knowledgeEditor.advanced.multimodal.cos.pathPrefixDescription') }}</p>
            </div>
            <div class="setting-control">
              <t-input
                v-model="localMultimodal.cos.pathPrefix"
                :placeholder="$t('knowledgeEditor.advanced.multimodal.cos.pathPrefixPlaceholder')"
                @change="handleConfigChange"
                style="width: 280px;"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- Knowledge graph extraction -->
      <div class="setting-row">
        <div class="setting-info">
          <label>{{ $t('knowledgeEditor.advanced.graph.label') }}</label>
          <p class="desc">{{ $t('knowledgeEditor.advanced.graph.description') }}</p>
          <!-- Warning message when graph database is not enabled -->
          <t-alert
            v-if="!isGraphDatabaseEnabled"
            theme="warning"
            style="margin-top: 8px;"
          >
            <template #message>
              <div>{{ $t('knowledgeEditor.advanced.graph.disabledWarning') }}</div>
              <t-link class="graph-guide-link" theme="primary" @click="handleOpenGraphGuide">
                {{ $t('knowledgeEditor.advanced.graph.howToEnable') }}
              </t-link>
            </template>
          </t-alert>
        </div>
        <div class="setting-control">
          <t-switch
            v-model="localNodeExtract.enabled"
            @change="handleNodeExtractToggle"
            :disabled="!isGraphDatabaseEnabled"
            size="large"
          />
        </div>
      </div>

      <!-- Knowledge graph configuration -->
      <div v-if="localNodeExtract.enabled && isGraphDatabaseEnabled" class="subsection">
        <div class="subsection-header">
          <h4>{{ $t('knowledgeEditor.advanced.graph.configTitle') }}</h4>
        </div>
        
        <div class="setting-row">
          <div class="setting-info">
            <label>{{ $t('knowledgeEditor.advanced.graph.promptLabel') }}</label>
            <p class="desc">{{ $t('knowledgeEditor.advanced.graph.promptDescription') }}</p>
          </div>
          <div class="setting-control">
            <t-textarea
              v-model="localNodeExtract.text"
              :placeholder="$t('knowledgeEditor.advanced.graph.promptPlaceholder')"
              :autosize="{ minRows: 3, maxRows: 6 }"
              @change="handleConfigChange"
              style="width: 280px;"
            />
          </div>
        </div>

        <div class="setting-row">
          <div class="setting-info">
            <label>{{ $t('knowledgeEditor.advanced.graph.tagsLabel') }}</label>
            <p class="desc">{{ $t('knowledgeEditor.advanced.graph.tagsDescription') }}</p>
          </div>
          <div class="setting-control">
            <t-tag-input
              v-model="localNodeExtract.tags"
              :placeholder="$t('knowledgeEditor.advanced.graph.tagsPlaceholder')"
              @change="handleConfigChange"
              style="width: 280px;"
            />
          </div>
        </div>

        <!-- Nodes Configuration -->
        <div class="subsection-header">
          <h4>{{ $t('knowledgeEditor.advanced.graph.nodesLabel') }}</h4>
          <p class="subsection-desc">{{ $t('knowledgeEditor.advanced.graph.nodesDescription') }}</p>
        </div>

        <div v-for="(node, index) in localNodeExtract.nodes" :key="index" class="config-item">
          <div class="config-item-header">
            <span class="config-item-title">节点 #{{ index + 1 }}</span>
            <t-button
              theme="danger"
              variant="text"
              size="small"
              @click="removeNode(index)"
            >
              {{ $t('knowledgeEditor.advanced.graph.deleteNode') }}
            </t-button>
          </div>
          
          <div class="config-item-body">
            <div class="config-field">
              <label>{{ $t('knowledgeEditor.advanced.graph.nodeNameLabel') }}</label>
              <t-input
                v-model="node.name"
                :placeholder="$t('knowledgeEditor.advanced.graph.nodeNamePlaceholder')"
                @change="handleConfigChange"
              />
            </div>
            
            <div class="config-field">
              <label>{{ $t('knowledgeEditor.advanced.graph.nodeChunksLabel') }}</label>
              <t-tag-input
                v-model="node.chunks"
                :placeholder="$t('knowledgeEditor.advanced.graph.nodeChunksPlaceholder')"
                @change="handleConfigChange"
              />
            </div>
            
            <div class="config-field">
              <label>{{ $t('knowledgeEditor.advanced.graph.nodeAttributesLabel') }}</label>
              <t-tag-input
                v-model="node.attributes"
                :placeholder="$t('knowledgeEditor.advanced.graph.nodeAttributesPlaceholder')"
                @change="handleConfigChange"
              />
            </div>
          </div>
        </div>

        <t-button
          theme="primary"
          variant="outline"
          @click="addNode"
          style="margin-top: 12px;"
        >
          <template #icon>
            <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
              <path d="M8 2a1 1 0 011 1v4h4a1 1 0 110 2H9v4a1 1 0 11-2 0V9H3a1 1 0 110-2h4V3a1 1 0 011-1z"/>
            </svg>
          </template>
          {{ $t('knowledgeEditor.advanced.graph.addNode') }}
        </t-button>

        <!-- Relations Configuration -->
        <div class="subsection-header" style="margin-top: 24px;">
          <h4>{{ $t('knowledgeEditor.advanced.graph.relationsLabel') }}</h4>
          <p class="subsection-desc">{{ $t('knowledgeEditor.advanced.graph.relationsDescription') }}</p>
        </div>

        <div v-for="(relation, index) in localNodeExtract.relations" :key="index" class="config-item">
          <div class="config-item-header">
            <span class="config-item-title">关系 #{{ index + 1 }}</span>
            <t-button
              theme="danger"
              variant="text"
              size="small"
              @click="removeRelation(index)"
            >
              {{ $t('knowledgeEditor.advanced.graph.deleteRelation') }}
            </t-button>
          </div>
          
          <div class="config-item-body">
            <div class="config-field">
              <label>{{ $t('knowledgeEditor.advanced.graph.relationNode1Label') }}</label>
              <t-input
                v-model="relation.node1"
                :placeholder="$t('knowledgeEditor.advanced.graph.relationNode1Placeholder')"
                @change="handleConfigChange"
              />
            </div>
            
            <div class="config-field">
              <label>{{ $t('knowledgeEditor.advanced.graph.relationNode2Label') }}</label>
              <t-input
                v-model="relation.node2"
                :placeholder="$t('knowledgeEditor.advanced.graph.relationNode2Placeholder')"
                @change="handleConfigChange"
              />
            </div>
            
            <div class="config-field">
              <label>{{ $t('knowledgeEditor.advanced.graph.relationTypeLabel') }}</label>
              <t-input
                v-model="relation.type"
                :placeholder="$t('knowledgeEditor.advanced.graph.relationTypePlaceholder')"
                @change="handleConfigChange"
              />
            </div>
          </div>
        </div>

        <t-button
          theme="primary"
          variant="outline"
          @click="addRelation"
          style="margin-top: 12px;"
        >
          <template #icon>
            <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
              <path d="M8 2a1 1 0 011 1v4h4a1 1 0 110 2H9v4a1 1 0 11-2 0V9H3a1 1 0 110-2h4V3a1 1 0 011-1z"/>
            </svg>
          </template>
          {{ $t('knowledgeEditor.advanced.graph.addRelation') }}
        </t-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import ModelSelector from '@/components/ModelSelector.vue'
import { useUIStore } from '@/stores/ui'
import { getSystemInfo } from '@/api/system'

const uiStore = useUIStore()

interface MultimodalConfig {
  enabled: boolean
  storageType: 'minio' | 'cos'
  vllmModelId?: string
  minio: {
    bucketName: string
    useSSL: boolean
    pathPrefix: string
  }
  cos: {
    secretId: string
    secretKey: string
    region: string
    bucketName: string
    appId: string
    pathPrefix: string
  }
}

interface NodeExtractConfig {
  enabled: boolean
  text: string
  tags: string[]
  nodes: Array<{
    name: string
    chunks: string[]
    attributes: string[]
  }>
  relations: Array<{
    node1: string
    node2: string
    type: string
  }>
}

interface Props {
  multimodal: MultimodalConfig
  nodeExtract: NodeExtractConfig
  allModels?: any[]
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:multimodal': [value: MultimodalConfig]
  'update:nodeExtract': [value: NodeExtractConfig]
}>()

const localMultimodal = ref<MultimodalConfig>({ ...props.multimodal })
const localNodeExtract = ref<NodeExtractConfig>({ 
  ...props.nodeExtract,
  nodes: props.nodeExtract.nodes || [],
  relations: props.nodeExtract.relations || []
})

const vllmSelectorRef = ref()
const isGraphDatabaseEnabled = ref(false)
const isMinioEnabled = ref(false)

// Check system status on mount
onMounted(async () => {
  try {
    const systemInfo = await getSystemInfo()
    
    // Check graph database status
    if (systemInfo.data?.graph_database_engine) {
      // Check if graph database is enabled
      // Enabled if it's "Neo4j" or any other non-empty value that's not a disabled indicator
      const engine = systemInfo.data.graph_database_engine.trim()
      const disabledIndicators = ['未启用', '未配置', 'Unknown', 'Неизвестно', '']
      isGraphDatabaseEnabled.value = !disabledIndicators.includes(engine) && engine.length > 0
      
      // If graph database is disabled, also disable node extract
      if (!isGraphDatabaseEnabled.value && localNodeExtract.value.enabled) {
        localNodeExtract.value.enabled = false
        emit('update:nodeExtract', localNodeExtract.value)
      }
    } else {
      // No graph database engine info, assume disabled
      isGraphDatabaseEnabled.value = false
      if (localNodeExtract.value.enabled) {
        localNodeExtract.value.enabled = false
        emit('update:nodeExtract', localNodeExtract.value)
      }
    }
    
    // Check MinIO status
    isMinioEnabled.value = systemInfo.data?.minio_enabled === true
    
    // If MinIO is not enabled and storage type is minio, switch to cos
    if (!isMinioEnabled.value && localMultimodal.value.storageType === 'minio') {
      localMultimodal.value.storageType = 'cos'
      emit('update:multimodal', localMultimodal.value)
    }
  } catch (error) {
    console.error('Failed to fetch system info:', error)
    // Default to disabled if we can't fetch the info
    isGraphDatabaseEnabled.value = false
    isMinioEnabled.value = false
    if (localNodeExtract.value.enabled) {
      localNodeExtract.value.enabled = false
      emit('update:nodeExtract', localNodeExtract.value)
    }
    // If MinIO status unknown and storage type is minio, switch to cos
    if (localMultimodal.value.storageType === 'minio') {
      localMultimodal.value.storageType = 'cos'
      emit('update:multimodal', localMultimodal.value)
    }
  }
})

// Watch for prop changes
watch(() => props.multimodal, (newVal) => {
  localMultimodal.value = { ...newVal }
}, { deep: true })

watch(() => props.nodeExtract, (newVal) => {
  localNodeExtract.value = { 
    ...newVal,
    nodes: newVal.nodes || [],
    relations: newVal.relations || []
  }
}, { deep: true })

// Handle multimodal toggle
const handleMultimodalToggle = () => {
  // Reset related configuration when multimodal is disabled
  if (!localMultimodal.value.enabled) {
    localMultimodal.value.vllmModelId = ''
    localMultimodal.value.minio = {
      bucketName: '',
      useSSL: false,
      pathPrefix: ''
    }
    localMultimodal.value.cos = {
      secretId: '',
      secretKey: '',
      region: '',
      bucketName: '',
      appId: '',
      pathPrefix: ''
    }
  }
  emit('update:multimodal', localMultimodal.value)
}

// Handle storage type change
const handleStorageTypeChange = () => {
  // Prevent switching to minio if it's not enabled
  if (localMultimodal.value.storageType === 'minio' && !isMinioEnabled.value) {
    localMultimodal.value.storageType = 'cos'
  }
  emit('update:multimodal', localMultimodal.value)
}

// Handle VLLM model change
const handleVLLMChange = (modelId: string) => {
  localMultimodal.value.vllmModelId = modelId
  emit('update:multimodal', localMultimodal.value)
}

// Navigate to model management when adding models
const handleAddModel = (subSection: string) => {
  uiStore.openSettings('models', subSection)
}

const graphGuideUrl =
  import.meta.env.VITE_KG_GUIDE_URL ||
  'https://github.com/Tencent/WeKnora/blob/main/docs/%E5%BC%80%E5%90%AF%E7%9F%A5%E8%AF%86%E5%9B%BE%E8%B0%B1%E5%8A%9F%E8%83%BD.md'

// Handle knowledge graph toggle
const handleNodeExtractToggle = () => {
  // Prevent enabling if graph database is not enabled
  if (!isGraphDatabaseEnabled.value) {
    localNodeExtract.value.enabled = false
    return
  }
  emit('update:nodeExtract', localNodeExtract.value)
}

// Open guide documentation to show how to enable graph database
const handleOpenGraphGuide = () => {
  window.open(graphGuideUrl, '_blank', 'noopener')
}

// Handle configuration change
const handleConfigChange = () => {
  emit('update:multimodal', localMultimodal.value)
  emit('update:nodeExtract', localNodeExtract.value)
}

// Add a new node
const addNode = () => {
  if (!localNodeExtract.value.nodes) {
    localNodeExtract.value.nodes = []
  }
  localNodeExtract.value.nodes.push({
    name: '',
    chunks: [],
    attributes: []
  })
  handleConfigChange()
}

// Remove a node
const removeNode = (index: number) => {
  localNodeExtract.value.nodes.splice(index, 1)
  handleConfigChange()
}

// Add a new relation
const addRelation = () => {
  if (!localNodeExtract.value.relations) {
    localNodeExtract.value.relations = []
  }
  localNodeExtract.value.relations.push({
    node1: '',
    node2: '',
    type: ''
  })
  handleConfigChange()
}

// Remove a relation
const removeRelation = (index: number) => {
  localNodeExtract.value.relations.splice(index, 1)
  handleConfigChange()
}

// The allModels prop keeps model options in sync
</script>

<style lang="less" scoped>
.kb-advanced-settings {
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
}

.setting-info {
  flex: 1;
  max-width: 65%;
  padding-right: 24px;

  label {
    font-size: 15px;
    font-weight: 500;
    color: #333333;
    display: block;
    margin-bottom: 4px;
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
  min-width: 280px;
  display: flex;
  justify-content: flex-end;
  align-items: center;
}

.graph-guide-link {
  display: inline-block;
  margin-top: 8px;
}

.subsection {
  padding: 16px 20px;
  margin: 12px 0 0 0;
  background: #f8fafb;
  border-radius: 8px;
  border-left: 3px solid #07C05F;
  position: relative;
}

.subsection-header {
  margin: 16px 0 8px 0;
  
  &:first-child {
    margin-top: 0;
  }
  
  h4 {
    font-size: 15px;
    font-weight: 600;
    color: #333333;
    margin: 0;
    padding-left: 8px;
    border-left: 2px solid #07C05F;
    
    .required {
      color: #e34d59;
      margin-left: 4px;
    }
  }
}

.required {
  color: #e34d59;
  margin-left: 2px;
  font-weight: 500;
}

.storage-config {
  margin-top: 8px;
}

.config-item {
  background: #ffffff;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  padding: 16px;
  margin-bottom: 12px;
}

.config-item-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid #f0f0f0;
}

.config-item-title {
  font-size: 14px;
  font-weight: 600;
  color: #333333;
}

.config-item-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.config-field {
  display: flex;
  flex-direction: column;
  gap: 6px;

  label {
    font-size: 13px;
    font-weight: 500;
    color: #555555;
  }
}

.subsection-desc {
  font-size: 13px;
  color: #666666;
  margin: 4px 0 8px 8px;
  line-height: 1.5;
}

</style>

