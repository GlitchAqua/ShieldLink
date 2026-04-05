<template>
  <div>
    <h3 style="margin:0 0 16px">解密服务器</h3>
    <div style="display:flex;justify-content:flex-end;margin-bottom:12px;gap:8px">
      <el-button type="warning" @click="handleCheckInstallAll('decrypt')" :loading="checkingAllDecrypt">全部检测安装</el-button>
      <el-button type="primary" @click="openAdd('decrypt')">添加解密服务器</el-button>
    </div>
    <el-table :data="decryptServers" border stripe style="margin-bottom:32px">
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="name" label="名称" width="150" />
      <el-table-column prop="address" label="地址" width="200" />
      <el-table-column prop="admin_addr" label="管理地址" width="160" />
      <el-table-column prop="status" label="同步状态" width="100">
        <template #default="{row}"><el-tag :type="row.status==='synced'?'success':row.status==='error'?'danger':'info'" size="small">{{ statusMap[row.status] || row.status }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="install_status" label="安装状态" width="110">
        <template #default="{row}"><el-tag :type="installStatusType(row.install_status)" size="small">{{ installStatusMap[row.install_status] || row.install_status || '未知' }}</el-tag></template>
      </el-table-column>
      <el-table-column label="检测频率" width="100">
        <template #default="{row}">{{ intervalLabel(row.check_interval) }}</template>
      </el-table-column>
      <el-table-column label="上次检测" width="160">
        <template #default="{row}"><span style="font-size:12px">{{ row.last_checked_at ? formatTime(row.last_checked_at) : '-' }}</span></template>
      </el-table-column>
      <el-table-column label="操作" min-width="300">
        <template #default="{row}">
          <el-button size="small" type="warning" @click="handleCheckInstall('decrypt', row)" :loading="row._checking">检测安装</el-button>
          <el-button size="small" @click="handleSync(row)">同步</el-button>
          <el-button size="small" @click="openEdit('decrypt', row)">编辑</el-button>
          <el-button size="small" type="danger" @click="handleDelete('decrypt', row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <h3 style="margin:0 0 16px">合流服务器</h3>
    <div style="display:flex;justify-content:flex-end;margin-bottom:12px;gap:8px">
      <el-button type="warning" @click="handleCheckInstallAll('merge')" :loading="checkingAllMerge">全部检测安装</el-button>
      <el-button type="primary" @click="openAdd('merge')">添加合流服务器</el-button>
    </div>
    <el-table :data="mergeServers" border stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="name" label="名称" width="150" />
      <el-table-column prop="address" label="地址" width="200" />
      <el-table-column prop="admin_addr" label="管理地址" width="160" />
      <el-table-column prop="install_status" label="安装状态" width="110">
        <template #default="{row}"><el-tag :type="installStatusType(row.install_status)" size="small">{{ installStatusMap[row.install_status] || row.install_status || '未知' }}</el-tag></template>
      </el-table-column>
      <el-table-column label="检测频率" width="100">
        <template #default="{row}">{{ intervalLabel(row.check_interval) }}</template>
      </el-table-column>
      <el-table-column label="上次检测" width="160">
        <template #default="{row}"><span style="font-size:12px">{{ row.last_checked_at ? formatTime(row.last_checked_at) : '-' }}</span></template>
      </el-table-column>
      <el-table-column label="操作" min-width="260">
        <template #default="{row}">
          <el-button size="small" type="warning" @click="handleCheckInstall('merge', row)" :loading="row._checking">检测安装</el-button>
          <el-button size="small" @click="openEdit('merge', row)">编辑</el-button>
          <el-button size="small" type="danger" @click="handleDelete('merge', row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="showForm" :title="formTitle" width="500px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="地址">
          <el-input v-model="form.address" placeholder="1.2.3.4:19443" />
          <div style="font-size:12px;color:#909399;margin-top:4px">管理地址和令牌会根据此地址自动生成</div>
        </el-form-item>
        <el-form-item label="SSH密码">
          <el-input v-model="form.ssh_password" type="password" show-password placeholder="用于自动检测和安装" />
        </el-form-item>
        <el-form-item label="检测频率">
          <el-select v-model="form.check_interval">
            <el-option :value="0" label="不自动检测" />
            <el-option :value="10" label="每 10 秒" />
            <el-option :value="30" label="每 30 秒" />
            <el-option :value="60" label="每 1 分钟" />
            <el-option :value="300" label="每 5 分钟" />
            <el-option :value="600" label="每 10 分钟" />
            <el-option :value="1800" label="每 30 分钟" />
            <el-option :value="3600" label="每 1 小时" />
          </el-select>
          <div style="font-size:12px;color:#909399;margin-top:4px">自动检测进程是否运行，未运行则自动重新安装</div>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showForm=false">取消</el-button>
        <el-button type="primary" @click="handleSubmit">保存</el-button>
      </template>
    </el-dialog>

    <!-- Check Install Result Dialog -->
    <el-dialog v-model="showResult" title="检测安装结果" width="550px">
      <el-descriptions :column="1" border>
        <el-descriptions-item label="安装状态">
          <el-tag :type="resultData.installed ? 'success' : 'danger'">{{ resultData.installed ? '已安装' : '未安装' }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="执行动作">{{ actionMap[resultData.action] || resultData.action }}</el-descriptions-item>
        <el-descriptions-item label="错误信息" v-if="resultData.error">
          <span style="color:#F56C6C">{{ resultData.error }}</span>
        </el-descriptions-item>
      </el-descriptions>
      <div v-if="resultData.output" style="margin-top:12px">
        <div style="font-weight:bold;margin-bottom:6px">输出详情:</div>
        <el-input type="textarea" :model-value="resultData.output" :rows="6" readonly />
      </div>
      <template #footer>
        <el-button @click="showResult=false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import api from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const decryptServers = ref([])
const mergeServers = ref([])
const showForm = ref(false)
const formType = ref('decrypt')
const editing = ref(false)
const editId = ref(null)
const checkingAllDecrypt = ref(false)
const checkingAllMerge = ref(false)
const showResult = ref(false)
const resultData = ref({})

const defaultForm = {
  name: '', address: '', enabled: true,
  ssh_password: '', check_interval: 0
}

const form = reactive({ ...defaultForm })
const formTitle = computed(() => `${editing.value?'编辑':'添加'}${formType.value==='decrypt'?'解密':'合流'}服务器`)

const statusMap = { synced: '已同步', error: '错误', unknown: '未知', unreachable: '不可达' }
const installStatusMap = { installed: '已安装', not_installed: '未安装', error: '检测失败', unknown: '未知' }
const actionMap = { already_installed: '已安装，无需操作', reinstalled: '已自动重新安装', install_failed: '自动安装失败' }

const intervalOptions = { 0: '关闭', 10: '10秒', 30: '30秒', 60: '1分钟', 300: '5分钟', 600: '10分钟', 1800: '30分钟', 3600: '1小时' }
function intervalLabel(v) {
  if (intervalOptions[v]) return intervalOptions[v]
  if (v <= 0) return '关闭'
  return v < 60 ? v + '秒' : Math.floor(v / 60) + '分钟'
}

function formatTime(t) {
  if (!t) return '-'
  const d = new Date(t)
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function installStatusType(s) {
  if (s === 'installed') return 'success'
  if (s === 'not_installed' || s === 'error') return 'danger'
  return 'info'
}

async function load() {
  decryptServers.value = (await api.get('/servers')).data
  mergeServers.value = (await api.get('/merge-servers')).data
}
onMounted(load)

function openAdd(type_) {
  Object.assign(form, { ...defaultForm })
  formType.value = type_; editing.value = false; showForm.value = true
}

function openEdit(type_, row) {
  Object.assign(form, { ...defaultForm, ...row })
  formType.value = type_; editing.value = true; editId.value = row.id; showForm.value = true
}

async function handleSubmit() {
  const ep = formType.value === 'decrypt' ? '/servers' : '/merge-servers'
  if (editing.value) await api.put(`${ep}/${editId.value}`, form)
  else await api.post(ep, form)
  ElMessage.success('已保存'); showForm.value = false; load()
}

async function handleDelete(type_, row) {
  await ElMessageBox.confirm(`确定删除「${row.name}」？`, '确认')
  const ep = type_ === 'decrypt' ? '/servers' : '/merge-servers'
  await api.delete(`${ep}/${row.id}`)
  ElMessage.success('已删除'); load()
}

async function handleSync(row) {
  try {
    await api.post(`/servers/${row.id}/sync`)
    ElMessage.success('同步成功')
    load()
  } catch {}
}

async function handleCheckInstall(type_, row) {
  row._checking = true
  const ep = type_ === 'decrypt' ? '/servers' : '/merge-servers'
  try {
    const res = await api.post(`${ep}/${row.id}/check-install`)
    resultData.value = res.data
    showResult.value = true
    if (res.data.installed) {
      ElMessage.success(`${row.name}: ${res.data.action === 'reinstalled' ? '已自动重新安装' : '已安装'}`)
    } else {
      ElMessage.warning(`${row.name}: 未安装 - ${res.data.error || ''}`)
    }
    load()
  } catch (e) {
    ElMessage.error(`检测失败: ${e.response?.data?.error || e.message}`)
  } finally {
    row._checking = false
  }
}

async function handleCheckInstallAll(type_) {
  const loading = type_ === 'decrypt' ? checkingAllDecrypt : checkingAllMerge
  loading.value = true
  const ep = type_ === 'decrypt' ? '/servers' : '/merge-servers'
  try {
    const res = await api.post(`${ep}/check-install-all`)
    const results = res.data
    const names = Object.keys(results)
    if (names.length === 0) {
      ElMessage.info('没有已启用且配置了SSH的服务器')
    } else {
      const ok = names.filter(n => results[n].installed)
      const fail = names.filter(n => !results[n].installed)
      const reinstalled = names.filter(n => results[n].action === 'reinstalled')
      let msg = `检测完成: ${ok.length}/${names.length} 已安装`
      if (reinstalled.length > 0) msg += `，${reinstalled.length} 个已自动重装`
      if (fail.length > 0) msg += `，${fail.length} 个未安装`
      ElMessage({ message: msg, type: fail.length > 0 ? 'warning' : 'success', duration: 5000 })
    }
    load()
  } catch (e) {
    ElMessage.error(`批量检测失败: ${e.response?.data?.error || e.message}`)
  } finally {
    loading.value = false
  }
}
</script>
