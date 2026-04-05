<template>
  <div>
    <h3 style="margin:0 0 20px">仪表盘</h3>
    <el-row :gutter="16">
      <el-col :span="4" v-for="(val, key) in stats" :key="key">
        <el-card shadow="hover">
          <el-statistic :title="labels[key] || key" :value="val" />
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '../api'

const stats = ref({})
const labels = { upstreams: '上游面板', decrypt_servers: '解密服务器', merge_servers: '合流服务器', routes: '路由', rules: '装饰规则' }

onMounted(async () => {
  const { data } = await api.get('/dashboard')
  stats.value = data
})
</script>
