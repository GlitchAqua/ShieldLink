<template>
  <div style="display:flex;justify-content:center;align-items:center;height:100vh;background:#f5f5f5">
    <el-card style="width:380px">
      <template #header><h3 style="margin:0;text-align:center">ShieldLink 管理面板</h3></template>
      <el-form @submit.prevent="handleLogin">
        <el-form-item><el-input v-model="form.username" placeholder="用户名" prefix-icon="User" /></el-form-item>
        <el-form-item><el-input v-model="form.password" placeholder="密码" type="password" prefix-icon="Lock" show-password /></el-form-item>
        <el-button type="primary" native-type="submit" :loading="loading" style="width:100%">登录</el-button>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import api from '../api'
import { ElMessage } from 'element-plus'

const router = useRouter()
const auth = useAuthStore()
const form = reactive({ username: '', password: '' })
const loading = ref(false)

async function handleLogin() {
  loading.value = true
  try {
    const { data } = await api.post('/auth/login', form)
    auth.setAuth(data.token, data.user)
    router.push('/')
  } catch { /* interceptor handles */ }
  finally { loading.value = false }
}
</script>
