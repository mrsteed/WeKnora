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
          </div>
          <div class="setting-control">
            <t-radio-group v-model="localMultimodal.storageType" @change="handleStorageTypeChange">
              <t-radio value="minio">{{ $t('knowledgeEditor.advanced.multimodal.storageTypeOptions.minio') }}</t-radio>
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
        </div>
        <div class="setting-control">
          <t-switch
            v-model="localNodeExtract.enabled"
            @change="handleNodeExtractToggle"
            size="large"
          />
        </div>
      </div>

      <!-- Knowledge graph configuration -->
      <div v-if="localNodeExtract.enabled" class="subsection">
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
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import ModelSelector from '@/components/ModelSelector.vue'
import { useUIStore } from '@/stores/ui'

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
const localNodeExtract = ref<NodeExtractConfig>({ ...props.nodeExtract })

const vllmSelectorRef = ref()

// Watch for prop changes
watch(() => props.multimodal, (newVal) => {
  localMultimodal.value = { ...newVal }
}, { deep: true })

watch(() => props.nodeExtract, (newVal) => {
  localNodeExtract.value = { ...newVal }
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

// Handle knowledge graph toggle
const handleNodeExtractToggle = () => {
  emit('update:nodeExtract', localNodeExtract.value)
}

// Handle configuration change
const handleConfigChange = () => {
  emit('update:multimodal', localMultimodal.value)
  emit('update:nodeExtract', localNodeExtract.value)
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
</style>

