<template>
  <div class="login-layout">
    <!-- Animated Dark Background -->
    <div class="animated-bg">
      <!-- Floating Orbs -->
      <div class="orb orb-1"></div>
      <div class="orb orb-2"></div>
      <div class="orb orb-3"></div>

      <!-- Neural Network Connection Lines -->
      <svg class="neural-lines" viewBox="0 0 100 100" preserveAspectRatio="none">
        <line class="conn-line cl-1" x1="15" y1="20" x2="35" y2="30" />
        <line class="conn-line cl-2" x1="35" y1="30" x2="55" y2="22" />
        <line class="conn-line cl-3" x1="55" y1="22" x2="75" y2="18" />
        <line class="conn-line cl-4" x1="10" y1="45" x2="30" y2="50" />
        <line class="conn-line cl-5" x1="30" y1="50" x2="50" y2="55" />
        <line class="conn-line cl-6" x1="50" y1="55" x2="70" y2="48" />
        <line class="conn-line cl-7" x1="70" y1="48" x2="90" y2="42" />
        <line class="conn-line cl-8" x1="12" y1="70" x2="28" y2="75" />
        <line class="conn-line cl-9" x1="28" y1="75" x2="48" y2="80" />
        <line class="conn-line cl-10" x1="48" y1="80" x2="68" y2="72" />
        <line class="conn-line cl-11" x1="20" y1="20" x2="15" y2="45" />
        <line class="conn-line cl-12" x1="35" y1="30" x2="30" y2="50" />
        <line class="conn-line cl-13" x1="55" y1="22" x2="50" y2="55" />
        <line class="conn-line cl-14" x1="70" y1="48" x2="68" y2="72" />
      </svg>

      <!-- Neural Nodes -->
      <div class="n-node nn-1"></div>
      <div class="n-node nn-2"></div>
      <div class="n-node nn-3"></div>
      <div class="n-node nn-4"></div>
      <div class="n-node nn-5"></div>
      <div class="n-node nn-6"></div>
      <div class="n-node nn-7"></div>
      <div class="n-node nn-8"></div>
      <div class="n-node nn-9"></div>
      <div class="n-node nn-10"></div>
    </div>

    <!-- Header Logo -->
    <div class="header-logo">
      <span class="logo-text">IntraMind</span>
      <span class="logo-divider">·</span>
      <span class="logo-sub">智枢</span>
    </div>

    <!-- Left Brand Showcase -->
    <div class="showcase-section">
      <div class="showcase-content">
        <!-- Neural Network Icon -->
        <div class="brand-icon-wrapper">
          <svg class="brand-icon" viewBox="0 0 200 200" fill="none">
            <!-- Outer Glow -->
            <ellipse cx="100" cy="100" rx="100" ry="100" fill="#6C3AFF" opacity="0.06" />
            <!-- Mid Ring -->
            <ellipse cx="100" cy="100" rx="80" ry="80" stroke="#00D4FF" stroke-width="1.5" opacity="0.3" fill="none" />
            <!-- Inner Ring -->
            <ellipse cx="100" cy="100" rx="55" ry="55" stroke="#6C3AFF" stroke-width="1" opacity="0.35" fill="none" />
            <!-- Core -->
            <ellipse cx="100" cy="100" rx="25" ry="25" fill="#6C3AFF" opacity="0.5" />
            <ellipse cx="100" cy="100" rx="16" ry="16" fill="#00D4FF" opacity="0.85" />
            <ellipse cx="100" cy="100" rx="8" ry="8" fill="#FFFFFF" opacity="0.95" />
            <!-- Connection Lines -->
            <line x1="50" y1="50" x2="84" y2="84" stroke="#00D4FF" stroke-width="1.5" opacity="0.45" />
            <line x1="150" y1="50" x2="116" y2="84" stroke="#6C3AFF" stroke-width="1.5" opacity="0.45" />
            <line x1="50" y1="150" x2="84" y2="116" stroke="#6C3AFF" stroke-width="1.5" opacity="0.45" />
            <line x1="150" y1="150" x2="116" y2="116" stroke="#00D4FF" stroke-width="1.5" opacity="0.45" />
            <line x1="100" y1="22" x2="100" y2="75" stroke="#00D4FF" stroke-width="1" opacity="0.35" />
            <line x1="100" y1="125" x2="100" y2="178" stroke="#00D4FF" stroke-width="1" opacity="0.35" />
            <line x1="22" y1="100" x2="75" y2="100" stroke="#6C3AFF" stroke-width="1" opacity="0.35" />
            <line x1="125" y1="100" x2="178" y2="100" stroke="#6C3AFF" stroke-width="1" opacity="0.35" />
            <!-- Nodes -->
            <circle cx="40" cy="45" r="7" fill="#00D4FF" />
            <circle cx="160" cy="45" r="7" fill="#6C3AFF" />
            <circle cx="40" cy="155" r="7" fill="#6C3AFF" />
            <circle cx="160" cy="155" r="7" fill="#00D4FF" />
            <circle cx="100" cy="18" r="6" fill="#00D4FF" />
            <circle cx="100" cy="182" r="6" fill="#00D4FF" />
            <circle cx="14" cy="100" r="6" fill="#6C3AFF" />
            <circle cx="186" cy="100" r="6" fill="#6C3AFF" />
            <!-- Orbital Rings -->
            <ellipse cx="100" cy="100" rx="95" ry="35" stroke="#00D4FF" stroke-width="0.8" opacity="0.15" fill="none"
              transform="rotate(-20 100 100)" />
            <ellipse cx="100" cy="100" rx="85" ry="28" stroke="#6C3AFF" stroke-width="0.8" opacity="0.12" fill="none"
              transform="rotate(30 100 100)" />
          </svg>
        </div>

        <!-- Accent Bar -->
        <div class="accent-bar"></div>

        <!-- Brand Title -->
        <h1 class="brand-title">IntraMind<span class="brand-cn">-智枢</span></h1>
        <p class="brand-tagline">{{ $t('platform.subtitle') }}</p>

        <!-- Feature Tags -->
        <div class="feature-tags">
          <span class="tag">{{ $t('platform.rag') }}</span>
          <span class="tag">{{ $t('platform.hybridSearch') }}</span>
          <span class="tag">{{ $t('platform.localDeploy') }}</span>
        </div>
      </div>
    </div>

    <!-- Right Form Section -->
    <div class="form-section">
      <div class="form-panel">
        <!-- Login Card -->
        <div class="form-card" v-if="!isRegisterMode">
          <div class="form-header">
            <h2 class="form-title">{{ $t('auth.login') }}</h2>
            <p class="form-welcome">{{ $t('auth.subtitle') }}</p>
          </div>

          <div class="form-content">
            <t-form ref="formRef" class="login-form" :data="formData" :rules="formRules" @submit="handleLogin" layout="vertical">
              <t-form-item :label="$t('auth.username')" name="account">
                <t-input v-model="formData.account" :placeholder="$t('auth.accountPlaceholder')" type="text" size="large"
                  :disabled="loading" />
              </t-form-item>

              <t-form-item :label="$t('auth.password')" name="password">
                <t-input v-model="formData.password" :placeholder="$t('auth.passwordPlaceholder')" type="password"
                  size="large" :disabled="loading" @keydown.enter="handleLogin" />
              </t-form-item>

              <div class="remember-row">
                <span class="form-spacer" aria-hidden="true"></span>
                <div class="remember-control">
                  <t-checkbox v-model="rememberAccount" :disabled="loading" @change="handleRememberAccountChange">
                    {{ $t('auth.rememberUsername') }}
                  </t-checkbox>
                  <t-checkbox v-model="rememberPassword" :disabled="loading" @change="handleRememberPasswordChange">
                    {{ $t('auth.rememberPassword') }}
                  </t-checkbox>
                </div>
              </div>

              <div class="submit-row">
                <span class="form-spacer" aria-hidden="true"></span>
                <t-button type="submit" theme="primary" size="large" block :loading="loading" class="submit-button">
                  {{ loading ? $t('auth.loggingIn') : $t('auth.login') }}
                </t-button>
              </div>
            </t-form>


            <div class="login-features">
              <div class="feature-item">
                <span class="feature-icon">✓</span>
                <span class="feature-text">{{ $t('platform.multimodalParsing') }}</span>
              </div>
              <div class="feature-item">
                <span class="feature-icon">✓</span>
                <span class="feature-text">{{ $t('platform.hybridSearchEngine') }}</span>
              </div>
              <div class="feature-item">
                <span class="feature-icon">✓</span>
                <span class="feature-text">{{ $t('platform.ragQandA') }}</span>
              </div>
            </div>
          </div>
        </div>

      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, nextTick, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { login, register } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

const router = useRouter()
const authStore = useAuthStore()
const { t } = useI18n()

const REMEMBERED_LOGIN_KEY = 'weknora_remembered_login'

// Form references
const formRef = ref()
const registerFormRef = ref()

// State management
const loading = ref(false)
const isRegisterMode = ref(false)
const rememberAccount = ref(false)
const rememberPassword = ref(false)

// Login form data
const formData = reactive<{ [key: string]: any }>({
  account: '',
  password: '',
})

// Register form data
const registerData = reactive<{ [key: string]: any }>({
  username: '',
  email: '',
  password: '',
  confirmPassword: ''
})

// Login form validation rules
const formRules = computed(() => ({
  account: [
    { required: true, message: t('auth.accountRequired'), type: 'error' },
  ],
  password: [
    { required: true, message: t('auth.passwordRequired'), type: 'error' },
    { min: 8, message: t('auth.passwordMinLength'), type: 'error' },
    { max: 32, message: t('auth.passwordMaxLength'), type: 'error' },
    { pattern: /[a-zA-Z]/, message: t('auth.passwordMustContainLetter'), type: 'error' },
    { pattern: /\d/, message: t('auth.passwordMustContainNumber'), type: 'error' }
  ]
}))

// Register form validation rules
const registerRules = computed(() => ({
  username: [
    { required: true, message: t('auth.usernameRequired'), type: 'error' },
    { min: 2, message: t('auth.usernameMinLength'), type: 'error' },
    { max: 20, message: t('auth.usernameMaxLength'), type: 'error' },
    {
      pattern: /^[a-zA-Z0-9_\u4e00-\u9fa5]+$/,
      message: t('auth.usernameInvalid'),
      type: 'error'
    }
  ],
  email: [
    { required: true, message: t('auth.emailRequired'), type: 'error' },
    { email: true, message: t('auth.emailInvalid'), type: 'error' }
  ],
  password: [
    { required: true, message: t('auth.passwordRequired'), type: 'error' },
    { min: 8, message: t('auth.passwordMinLength'), type: 'error' },
    { max: 32, message: t('auth.passwordMaxLength'), type: 'error' },
    { pattern: /[a-zA-Z]/, message: t('auth.passwordMustContainLetter'), type: 'error' },
    { pattern: /\d/, message: t('auth.passwordMustContainNumber'), type: 'error' }
  ],
  confirmPassword: [
    { required: true, message: t('auth.confirmPasswordRequired'), type: 'error' },
    {
      validator: (val: string) => val === registerData.password,
      message: t('auth.passwordMismatch'),
      type: 'error'
    }
  ]
}))

// Toggle login/register mode
const toggleMode = () => {
  isRegisterMode.value = !isRegisterMode.value
  Object.keys(registerData).forEach(key => {
    (registerData as any)[key] = ''
  })
}

const clearRememberedCredentials = () => {
  localStorage.removeItem(REMEMBERED_LOGIN_KEY)
}

const saveRememberedCredentials = () => {
  localStorage.setItem(REMEMBERED_LOGIN_KEY, JSON.stringify({
    rememberAccount: rememberAccount.value,
    rememberPassword: rememberPassword.value,
    account: rememberAccount.value ? formData.account : '',
    password: rememberPassword.value ? formData.password : '',
  }))
}

const loadRememberedCredentials = () => {
  const remembered = localStorage.getItem(REMEMBERED_LOGIN_KEY)
  if (!remembered) return

  try {
    const parsed = JSON.parse(remembered)
    const rememberedAccount = typeof parsed?.account === 'string'
      ? parsed.account
      : typeof parsed?.email === 'string'
        ? parsed.email
        : ''

    if (rememberedAccount) {
      formData.account = rememberedAccount
    }
    if (typeof parsed?.password === 'string') {
      formData.password = parsed.password
    }

    rememberAccount.value = parsed?.rememberAccount === true || !!rememberedAccount
    rememberPassword.value = parsed?.rememberPassword === true && typeof parsed?.password === 'string' && parsed.password.length > 0

    if (rememberPassword.value && !rememberAccount.value) {
      rememberAccount.value = true
    }
  } catch {
    clearRememberedCredentials()
  }
}

const handleRememberAccountChange = (checked: boolean | string | number) => {
  const enabled = !!checked
  if (!enabled && rememberPassword.value) {
    rememberPassword.value = false
  }
  if (!enabled) {
    clearRememberedCredentials()
  }
}

const handleRememberPasswordChange = (checked: boolean | string | number) => {
  const enabled = !!checked
  if (enabled && !rememberAccount.value) {
    rememberAccount.value = true
  }
  if (!enabled && !rememberAccount.value) {
    clearRememberedCredentials()
  }
}

// Handle login
const handleLogin = async () => {
  try {
    const valid = await formRef.value?.validate()
    if (!valid) return

    loading.value = true

    const response = await login({
      email: formData.account,
      password: formData.password,
    })

    if (response.success) {
      if (response.user && response.tenant && response.token) {
        authStore.setUser({
          id: response.user.id || '',
          username: response.user.username || '',
          email: response.user.email || '',
          avatar: response.user.avatar,
          tenant_id: String(response.tenant.id) || '',
          can_access_all_tenants: response.user.can_access_all_tenants || false,
          created_at: response.user.created_at || new Date().toISOString(),
          updated_at: response.user.updated_at || new Date().toISOString()
        })
        authStore.setToken(response.token)
        if (response.refresh_token) {
          authStore.setRefreshToken(response.refresh_token)
        }
        authStore.setTenant({
          id: String(response.tenant.id) || '',
          name: response.tenant.name || '',
          api_key: response.tenant.api_key || '',
          owner_id: response.user.id || '',
          created_at: response.tenant.created_at || new Date().toISOString(),
          updated_at: response.tenant.updated_at || new Date().toISOString()
        })
        authStore.setSelectedTenant(null, null)

        if (rememberAccount.value || rememberPassword.value) {
          saveRememberedCredentials()
        } else {
          clearRememberedCredentials()
        }
      }

      MessagePlugin.success(t('auth.loginSuccess'))
      await nextTick()
      router.replace('/platform/knowledge-bases')
    } else {
      MessagePlugin.error(response.message || t('auth.loginError'))
    }
  } catch (error: any) {
    console.error('登录错误:', error)
    MessagePlugin.error(error.message || t('auth.loginErrorRetry'))
  } finally {
    loading.value = false
  }
}

// Handle registration
const handleRegister = async () => {
  try {
    const valid = await registerFormRef.value?.validate()
    if (!valid) return

    loading.value = true

    const response = await register({
      username: registerData.username,
      email: registerData.email,
      password: registerData.password
    })

    if (response.success) {
      MessagePlugin.success(t('auth.registerSuccess'))
      isRegisterMode.value = false
      formData.account = registerData.email
      Object.keys(registerData).forEach(key => {
        (registerData as any)[key] = ''
      })
    } else {
      MessagePlugin.error(response.message || t('auth.registerFailed'))
    }
  } catch (error: any) {
    console.error('注册错误:', error)
    MessagePlugin.error(error.message || t('auth.registerError'))
  } finally {
    loading.value = false
  }
}

// Check if already logged in
onMounted(() => {
  loadRememberedCredentials()

  if (authStore.isLoggedIn) {
    router.replace('/platform/tenant/knowledge-bases')
  }
})
</script>

<style lang="less" scoped>
/* ═══════════════════════════════════════════════════
   IntraMind-智枢 Login Page — Dark Neural Theme
   Colors: Cyan #00D4FF · Purple #6C3AFF · Dark #0A0A0B
   ═══════════════════════════════════════════════════ */

.login-layout {
  display: flex;
  width: 100%;
  min-height: 100vh;
  overflow: hidden;
  position: relative;
  background: #0A0A0B;

  &::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background:
      radial-gradient(ellipse at 20% 40%, rgba(108, 58, 255, 0.08) 0%, transparent 60%),
      radial-gradient(ellipse at 80% 60%, rgba(0, 212, 255, 0.06) 0%, transparent 60%),
      radial-gradient(ellipse at 50% 100%, rgba(108, 58, 255, 0.04) 0%, transparent 50%);
    pointer-events: none;
    z-index: 0;
  }
}

.remember-row {
  display: grid;
  grid-template-columns: 88px minmax(0, 1fr);
  align-items: center;
  column-gap: 16px;
  margin: -2px 0 10px;

  .remember-control {
    min-width: 0;
    display: flex;
    flex-direction: row;
    align-items: flex-start;
    flex-wrap: nowrap;
    gap: 20px;
  }

  :deep(.t-checkbox) {
    color: rgba(255, 255, 255, 0.78);
    font-size: 13px;
    margin-right: 0;
    line-height: 1;
    white-space: nowrap;
  }

  :deep(.t-checkbox + .t-checkbox) {
    margin-left: 0;
  }
}

.submit-row {
  display: grid;
  grid-template-columns: 88px minmax(0, 1fr);
  column-gap: 16px;
  align-items: center;
}

.form-spacer {
  display: block;
}

/* ── Animated Background ── */
.animated-bg {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  pointer-events: none;
  z-index: 1;
  overflow: hidden;
}

.orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  animation: orbFloat 12s infinite ease-in-out;
}

.orb-1 {
  width: 400px;
  height: 400px;
  background: rgba(0, 212, 255, 0.06);
  top: -10%;
  left: 10%;
  animation-delay: 0s;
}

.orb-2 {
  width: 350px;
  height: 350px;
  background: rgba(108, 58, 255, 0.06);
  bottom: -5%;
  right: 15%;
  animation-delay: 4s;
}

.orb-3 {
  width: 300px;
  height: 300px;
  background: rgba(0, 212, 255, 0.04);
  top: 50%;
  left: 40%;
  animation-delay: 8s;
}

@keyframes orbFloat {

  0%,
  100% {
    transform: translate(0, 0) scale(1);
  }

  33% {
    transform: translate(30px, -20px) scale(1.05);
  }

  66% {
    transform: translate(-20px, 15px) scale(0.95);
  }
}

/* Neural network lines */
.neural-lines {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  opacity: 0.25;
}

.conn-line {
  stroke: rgba(0, 212, 255, 0.5);
  stroke-width: 1;
  stroke-dasharray: 5, 4;
  stroke-linecap: round;
  animation: lineFlow 8s infinite linear;
}

.cl-1 {
  animation-delay: 0s;
}

.cl-2 {
  animation-delay: 0.6s;
}

.cl-3 {
  animation-delay: 1.2s;
}

.cl-4 {
  animation-delay: 0.3s;
  stroke: rgba(108, 58, 255, 0.5);
}

.cl-5 {
  animation-delay: 0.9s;
  stroke: rgba(108, 58, 255, 0.5);
}

.cl-6 {
  animation-delay: 1.5s;
}

.cl-7 {
  animation-delay: 2.1s;
}

.cl-8 {
  animation-delay: 0.4s;
  stroke: rgba(108, 58, 255, 0.5);
}

.cl-9 {
  animation-delay: 1s;
}

.cl-10 {
  animation-delay: 1.6s;
  stroke: rgba(108, 58, 255, 0.5);
}

.cl-11 {
  animation-delay: 0.7s;
}

.cl-12 {
  animation-delay: 1.3s;
  stroke: rgba(108, 58, 255, 0.5);
}

.cl-13 {
  animation-delay: 0.5s;
}

.cl-14 {
  animation-delay: 1.8s;
  stroke: rgba(108, 58, 255, 0.5);
}

@keyframes lineFlow {
  0% {
    stroke-dashoffset: 0;
  }

  100% {
    stroke-dashoffset: 18;
  }
}

/* Neural Nodes */
.n-node {
  position: absolute;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #00D4FF;
  box-shadow: 0 0 8px rgba(0, 212, 255, 0.6);
  animation: nodePulse 3s infinite ease-in-out;
}

.nn-1 {
  top: 20%;
  left: 15%;
  animation-delay: 0s;
}

.nn-2 {
  top: 30%;
  left: 35%;
  animation-delay: 0.5s;
  background: #6C3AFF;
  box-shadow: 0 0 8px rgba(108, 58, 255, 0.6);
}

.nn-3 {
  top: 22%;
  left: 55%;
  animation-delay: 1s;
}

.nn-4 {
  top: 18%;
  left: 75%;
  animation-delay: 1.5s;
  background: #6C3AFF;
  box-shadow: 0 0 8px rgba(108, 58, 255, 0.6);
}

.nn-5 {
  top: 45%;
  left: 10%;
  animation-delay: 2s;
  background: #6C3AFF;
  box-shadow: 0 0 8px rgba(108, 58, 255, 0.6);
}

.nn-6 {
  top: 50%;
  left: 30%;
  animation-delay: 0.8s;
}

.nn-7 {
  top: 55%;
  left: 50%;
  animation-delay: 1.3s;
  background: #6C3AFF;
  box-shadow: 0 0 8px rgba(108, 58, 255, 0.6);
}

.nn-8 {
  top: 48%;
  left: 70%;
  animation-delay: 1.8s;
}

.nn-9 {
  top: 70%;
  left: 28%;
  animation-delay: 0.3s;
}

.nn-10 {
  top: 80%;
  left: 48%;
  animation-delay: 2.3s;
  background: #6C3AFF;
  box-shadow: 0 0 8px rgba(108, 58, 255, 0.6);
}

@keyframes nodePulse {

  0%,
  100% {
    transform: scale(1);
    opacity: 0.6;
  }

  50% {
    transform: scale(1.4);
    opacity: 1;
  }
}

/* ── Header Logo ── */
.header-logo {
  position: fixed;
  top: 32px;
  left: 50px;
  z-index: 100;
  display: flex;
  align-items: baseline;
  gap: 4px;

  .logo-text {
    font-family: 'Inter', sans-serif;
    font-size: 22px;
    font-weight: 700;
    color: #FFFFFF;
    letter-spacing: 1px;
  }

  .logo-divider {
    color: rgba(255, 255, 255, 0.3);
    font-size: 20px;
    margin: 0 2px;
  }

  .logo-sub {
    font-family: "PingFang SC", sans-serif;
    font-size: 18px;
    font-weight: 500;
    background: linear-gradient(135deg, #00D4FF, #6C3AFF);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
  }
}

/* ── Header Links ── */
.header-links {
  position: fixed;
  top: 28px;
  right: 28px;
  display: flex;
  align-items: center;
  gap: 10px;
  z-index: 100;
}

/* ── Left Showcase Section ── */
.showcase-section {
  flex: 0 0 52%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 100px 30px 100px 50px;
  box-sizing: border-box;
  position: relative;
  z-index: 2;
}

.showcase-content {
  width: 100%;
  max-width: 500px;
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}

.brand-icon-wrapper {
  width: 220px;
  height: 220px;
  margin-bottom: 32px;
  animation: iconFloat 6s infinite ease-in-out;
  filter: drop-shadow(0 0 30px rgba(0, 212, 255, 0.2)) drop-shadow(0 0 60px rgba(108, 58, 255, 0.1));
}

.brand-icon {
  width: 100%;
  height: 100%;
}

@keyframes iconFloat {

  0%,
  100% {
    transform: translateY(0);
  }

  50% {
    transform: translateY(-12px);
  }
}

.accent-bar {
  width: 60px;
  height: 2px;
  border-radius: 2px;
  background: linear-gradient(90deg, #6C3AFF, #00D4FF);
  margin-bottom: 24px;
}

.brand-title {
  font-family: 'Inter', sans-serif;
  font-size: 44px;
  font-weight: 700;
  color: #FFFFFF;
  letter-spacing: 2px;
  margin: 0 0 12px 0;
  line-height: 1.2;

  .brand-cn {
    font-family: "PingFang SC", sans-serif;
    font-weight: 600;
    font-size: 36px;
    background: linear-gradient(135deg, #00D4FF, #6C3AFF);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
  }
}

.brand-tagline {
  font-size: 16px;
  color: #8B8B90;
  margin: 0 0 32px 0;
  font-family: "PingFang SC", sans-serif;
  letter-spacing: 2px;
  line-height: 1.6;
}

.feature-tags {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  justify-content: center;
}

.tag {
  display: inline-block;
  padding: 8px 20px;
  background: rgba(0, 212, 255, 0.08);
  backdrop-filter: blur(10px);
  border-radius: 20px;
  border: 1px solid rgba(0, 212, 255, 0.15);
  color: rgba(255, 255, 255, 0.85);
  font-size: 13px;
  font-weight: 500;
  font-family: "PingFang SC", sans-serif;
  transition: all 0.3s ease;

  &:hover {
    background: rgba(0, 212, 255, 0.15);
    border-color: rgba(0, 212, 255, 0.3);
    transform: translateY(-2px);
    box-shadow: 0 4px 12px rgba(0, 212, 255, 0.15);
  }
}

/* ── Right Form Section ── */
.form-section {
  flex: 0 0 48%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 50px 40px 30px;
  box-sizing: border-box;
  position: relative;
  z-index: 2;
}

.form-panel {
  width: 100%;
  max-width: 440px;
}

.form-card {
  background: rgba(255, 255, 255, 0.04);
  backdrop-filter: blur(24px);
  border-radius: 20px;
  padding: 40px;
  box-shadow:
    0 8px 32px rgba(0, 0, 0, 0.3),
    inset 0 0 0 1px rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
  animation: slideInRight 0.4s ease-out;
  width: 100%;
  box-sizing: border-box;
}

.form-header {
  text-align: center;
  margin-bottom: 32px;
}

.form-title {
  font-size: 26px;
  font-weight: 600;
  color: #FFFFFF;
  margin: 0 0 8px 0;
  font-family: 'Inter', "PingFang SC", sans-serif;
}

.form-welcome,
.form-subtitle {
  font-size: 14px;
  color: #8B8B90;
  margin: 0;
  font-family: "PingFang SC", sans-serif;
}

.form-content {

  :deep(.login-form) {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  :deep(.login-form .t-form-item) {
    display: grid;
    grid-template-columns: 88px minmax(0, 1fr);
    column-gap: 16px;
    align-items: start;
    margin-bottom: 0;
  }

  /* Form labels - force white text */
  :deep(.t-form-item__label) {
    font-size: 14px !important;
    color: rgba(255, 255, 255, 0.85) !important;
    font-weight: 500;
    margin-bottom: 0;
    padding-top: 12px;
    font-family: "PingFang SC", sans-serif;
    display: block;
    text-align: left;
  }

  /* Required asterisk */
  :deep(.t-form__controls-content .t-form-item__label .t-form__label--required),
  :deep(.t-is-required > .t-form-item__label::before) {
    color: #00D4FF !important;
  }

  /* Input wrapper */
  :deep(.t-input) {
    border: 1px solid rgba(255, 255, 255, 0.15) !important;
    border-radius: 10px !important;
    background: rgba(255, 255, 255, 0.06) !important;
    transition: all 0.25s ease;

    &:focus-within {
      border-color: #00D4FF !important;
      box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.15), 0 0 20px rgba(0, 212, 255, 0.08) !important;
    }

    &:hover {
      border-color: rgba(0, 212, 255, 0.4) !important;
    }

    .t-input__inner {
      border: none !important;
      box-shadow: none !important;
      outline: none !important;
      background: transparent !important;
      font-size: 15px;
      font-family: "PingFang SC", sans-serif;
      color: #FFFFFF !important;

      &::placeholder {
        color: rgba(255, 255, 255, 0.35) !important;
      }

      &:focus {
        border: none !important;
        box-shadow: none !important;
        outline: none !important;
      }
    }

    .t-input__wrap {
      border: none !important;
      box-shadow: none !important;
      background: transparent !important;
    }

    /* Password eye icon */
    .t-input__suffix,
    .t-input__suffix-icon {
      color: rgba(255, 255, 255, 0.4) !important;

      &:hover {
        color: #00D4FF !important;
      }
    }
  }

  /* Error messages */
  :deep(.t-input__tips) {
    color: #ff6b6b !important;
  }

  :deep(.t-is-error .t-input) {
    border-color: rgba(255, 107, 107, 0.5) !important;
  }

  :deep(.t-form-item) {
    margin-bottom: 20px;

    &:last-child {
      margin-bottom: 0;
    }
  }

  :deep(.t-form-item__control) {
    width: 100%;
  }

  :deep(.t-form-item__controls) {
    min-width: 0;
  }
}

.submit-button {
  height: 48px;
  border-radius: 10px;
  font-size: 16px;
  font-weight: 600;
  font-family: "PingFang SC", sans-serif;
  margin: 0 0 16px 0;
  transition: all 0.3s ease;
  overflow: hidden;

  :deep(.t-button) {
    background: linear-gradient(135deg, #00D4FF 0%, #6C3AFF 100%) !important;
    border: none !important;
    color: #FFFFFF !important;

    &:hover {
      background: linear-gradient(135deg, #00BFEA 0%, #5A2FE0 100%) !important;
      transform: translateY(-2px);
      box-shadow: 0 8px 24px rgba(0, 212, 255, 0.25), 0 4px 12px rgba(108, 58, 255, 0.2);
    }

    &:active {
      transform: translateY(0);
    }
  }

  /* Also override the button element directly */
  :deep(.t-button--theme-primary) {
    background: linear-gradient(135deg, #00D4FF 0%, #6C3AFF 100%) !important;
    border-color: transparent !important;
  }
}

.form-footer {
  text-align: center;
  font-size: 14px;
  color: #8B8B90;
  font-family: "PingFang SC", sans-serif;
  margin-top: 16px;
  padding-bottom: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);

  .link-button {
    color: #00D4FF;
    text-decoration: none;
    margin-left: 4px;
    font-weight: 500;
    transition: all 0.2s;

    &:hover {
      color: #6C3AFF;
      text-decoration: underline;
    }
  }
}

.login-features {
  margin-top: 20px;

  .feature-item {
    display: flex;
    align-items: center;
    margin-bottom: 12px;
    font-size: 13px;
    color: rgba(255, 255, 255, 0.5);
    font-family: "PingFang SC", sans-serif;

    &:last-child {
      margin-bottom: 0;
    }

    .feature-icon {
      width: 20px;
      height: 20px;
      border-radius: 50%;
      background: rgba(0, 212, 255, 0.12);
      color: #00D4FF;
      display: flex;
      align-items: center;
      justify-content: center;
      font-size: 11px;
      font-weight: 700;
      margin-right: 10px;
      flex-shrink: 0;
    }
  }
}

/* ── Animations ── */
@keyframes slideInRight {
  from {
    opacity: 0;
    transform: translateX(24px);
  }

  to {
    opacity: 1;
    transform: translateX(0);
  }
}

/* ── Responsive ── */
@media (max-width: 1024px) {
  .header-logo {
    top: 26px;
    left: 40px;

    .logo-text {
      font-size: 18px;
    }

    .logo-sub {
      font-size: 15px;
    }
  }

  .header-links {
    top: 22px;
    right: 22px;
    gap: 8px;

    .link-text {
      display: none;
    }

    .header-link {
      padding: 10px;
      gap: 0;
    }
  }

  .brand-title {
    font-size: 36px;
  }

  .brand-title .brand-cn {
    font-size: 28px;
  }

  .brand-icon-wrapper {
    width: 180px;
    height: 180px;
  }
}

@media (max-width: 768px) {
  .login-layout {
    flex-direction: column;
  }

  .showcase-section {
    flex: 0 0 auto;
    min-height: 45vh;
    padding: 80px 24px 40px;
  }

  .showcase-content {
    max-width: 100%;
  }

  .header-logo {
    top: 22px;
    left: 30px;

    .logo-text {
      font-size: 16px;
    }

    .logo-sub {
      font-size: 14px;
    }
  }

  .brand-title {
    font-size: 30px;
  }

  .brand-title .brand-cn {
    font-size: 24px;
  }

  .brand-icon-wrapper {
    width: 160px;
    height: 160px;
    margin-bottom: 24px;
  }

  .brand-tagline {
    font-size: 14px;
    margin-bottom: 24px;
  }

  .feature-tags {
    margin-bottom: 0;
  }

  .form-section {
    flex: 0 0 auto;
    padding: 24px;
  }

  .header-links {
    top: 18px;
    right: 18px;
    gap: 8px;

    .link-text {
      display: inline;
    }

    .header-link {
      padding: 8px 12px;
      font-size: 12px;
    }
  }

  .form-card {
    padding: 32px 24px;
  }

  .form-title {
    font-size: 22px;
  }
}

@media (max-width: 480px) {
  .showcase-section {
    padding: 70px 20px 32px;
  }

  .header-logo {
    top: 18px;
    left: 20px;

    .logo-text {
      font-size: 15px;
    }

    .logo-sub {
      font-size: 13px;
    }
  }

  .brand-title {
    font-size: 26px;
  }

  .brand-title .brand-cn {
    font-size: 20px;
  }

  .brand-icon-wrapper {
    width: 130px;
    height: 130px;
  }

  .tag {
    font-size: 12px;
    padding: 6px 14px;
  }

  .form-section {
    padding: 20px;
  }

  .header-links {
    top: 14px;
    right: 14px;
    gap: 6px;

    .header-link {
      padding: 7px 10px;
      font-size: 11px;
    }
  }

  .form-card {
    padding: 28px 20px;
  }

  .form-header {
    margin-bottom: 24px;
  }

  .remember-row,
  .submit-row,
  .form-content :deep(.login-form .t-form-item) {
    grid-template-columns: 1fr;
    row-gap: 8px;
  }

  .remember-row .remember-control {
    flex-wrap: wrap;
    gap: 10px 16px;
  }

  .form-content :deep(.t-form-item__label) {
    padding-top: 0;
  }

  .form-spacer {
    display: none;
  }
}
</style>

<!-- Global TDesign Dark Theme Overrides (unscoped for specificity) -->
<style lang="less">
.login-layout {

  /* Force label text white */
  .t-form__label,
  .t-form-item__label,
  label.t-form__label {
    color: rgba(255, 255, 255, 0.85) !important;
  }

  /* Required asterisk */
  .t-form__label--required::before {
    color: #00D4FF !important;
  }

  /* Input container */
  .t-input {
    background: rgba(255, 255, 255, 0.06) !important;
    border: 1px solid rgba(255, 255, 255, 0.15) !important;
    border-radius: 10px !important;
    color: #FFFFFF !important;

    &:hover {
      border-color: rgba(0, 212, 255, 0.4) !important;
    }

    &.t-is-focused,
    &:focus-within {
      border-color: #00D4FF !important;
      box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.15) !important;
    }
  }

  .t-input__inner {
    color: #FFFFFF !important;
    background: transparent !important;
    caret-color: #00D4FF;

    &::placeholder {
      color: rgba(255, 255, 255, 0.35) !important;
    }
  }

  .t-input__wrap {
    background: transparent !important;
    border: none !important;
  }

  /* Password eye + suffix icons */
  .t-input__suffix,
  .t-input__suffix-icon,
  .t-icon {
    color: rgba(255, 255, 255, 0.45) !important;

    &:hover {
      color: #00D4FF !important;
    }
  }

  /* Submit button — cyan→purple gradient */
  .submit-button,
  .submit-button.t-button,
  button.t-button.t-button--theme-primary.submit-button,
  .t-button--theme-primary {
    background: linear-gradient(135deg, #00D4FF 0%, #6C3AFF 100%) !important;
    border: none !important;
    color: #FFFFFF !important;
    border-radius: 10px !important;
    height: 48px;
    font-size: 16px;
    font-weight: 600;
    transition: all 0.3s ease;

    &:hover {
      background: linear-gradient(135deg, #00BFEA 0%, #5A2FE0 100%) !important;
      transform: translateY(-2px);
      box-shadow: 0 8px 24px rgba(0, 212, 255, 0.25), 0 4px 12px rgba(108, 58, 255, 0.2);
    }

    &:active {
      transform: translateY(0);
    }

    .t-button__text {
      color: #FFFFFF !important;
    }
  }

  /* Error state */
  .t-is-error .t-input {
    border-color: rgba(255, 107, 107, 0.5) !important;
  }

  .t-input__tips--error {
    color: #ff6b6b !important;
  }
}
</style>
