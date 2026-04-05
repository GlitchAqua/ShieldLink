<template>
  <div>
    <div style="display:flex;justify-content:space-between;margin-bottom:16px">
      <h3 style="margin:0">装饰规则</h3>
      <el-button type="primary" @click="openAdd">添加规则</el-button>
    </div>
    <el-table :data="items" border stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="name" label="名称" width="150" />
      <el-table-column label="上游面板" width="120">
        <template #default="{row}">{{ row.upstream?.name || row.upstream_id }}</template>
      </el-table-column>
      <el-table-column prop="match_pattern" label="节点匹配" width="150" show-overflow-tooltip />
      <el-table-column prop="ua_pattern" label="UA 匹配" width="140" show-overflow-tooltip>
        <template #default="{row}"><code v-if="row.ua_pattern" style="font-size:11px">{{ row.ua_pattern }}</code><span v-else style="color:#ccc">全部</span></template>
      </el-table-column>
      <el-table-column label="路由" width="130">
        <template #default="{row}"><code style="font-size:11px">{{ row.route?.uuid || '-' }}</code></template>
      </el-table-column>
      <el-table-column prop="protocol" label="协议" width="70" />
      <el-table-column prop="priority" label="优先级" width="80" />
      <el-table-column label="启用" width="80">
        <template #default="{row}"><el-tag :type="row.enabled?'success':'danger'" size="small">{{ row.enabled?'是':'否' }}</el-tag></template>
      </el-table-column>
      <el-table-column label="操作" width="150">
        <template #default="{row}">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="showForm" :title="editing?'编辑规则':'添加规则'" width="600px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="上游面板">
          <el-select v-model="form.upstream_id" style="width:100%">
            <el-option v-for="u in upstreams" :key="u.id" :label="u.name" :value="u.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="节点匹配"><el-input v-model="form.match_pattern" placeholder="正则匹配节点名, 如 HK|香港" /></el-form-item>
        <el-form-item label="UA 匹配"><el-input v-model="form.ua_pattern" placeholder="留空=所有客户端, 如 verge|shieldlink" /></el-form-item>
        <el-form-item label="路由">
          <el-select v-model="form.route_id" style="width:100%">
            <el-option v-for="r in routes" :key="r.id" :label="`${r.uuid} → ${r.forward}`" :value="r.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="解密服务器">
          <el-select v-model="selectedServers" multiple style="width:100%">
            <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.address})`" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="form.protocol"><el-option label="TCP" value="tcp" /><el-option label="UDP" value="udp" /></el-select>
        </el-form-item>
        <el-form-item label="传输方式" v-if="form.protocol==='tcp'">
          <el-select v-model="form.transport"><el-option label="H2" value="h2" /><el-option label="WebSocket" value="ws" /></el-select>
        </el-form-item>
        <el-form-item label="MPTCP"><el-switch v-model="form.mptcp" /></el-form-item>
        <el-form-item label="聚合模式"><el-switch v-model="form.aggregate" /></el-form-item>
        <el-form-item label="合流服务器" v-if="form.aggregate">
          <el-select v-model="form.merge_server_id" clearable style="width:100%">
            <el-option v-for="m in mergeServers" :key="m.id" :label="`${m.name} (${m.address})`" :value="m.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="优先级"><el-input-number v-model="form.priority" :min="0" :max="9999" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showForm=false">取消</el-button>
        <el-button type="primary" @click="handleSubmit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import api from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const items = ref([])
const upstreams = ref([])
const routes = ref([])
const servers = ref([])
const mergeServers = ref([])
const showForm = ref(false)
const editing = ref(false)
const editId = ref(null)
const selectedServers = ref([])
const defaultForm = { name: '', upstream_id: null, match_pattern: '', ua_pattern: '', route_id: null, server_ids: '[]', protocol: 'tcp', transport: 'h2', mptcp: false, aggregate: false, merge_server_id: null, priority: 0, enabled: true }
const form = reactive({ ...defaultForm })

async function load() {
  [items.value, upstreams.value, routes.value, servers.value, mergeServers.value] = await Promise.all([
    api.get('/rules').then(r => r.data),
    api.get('/upstreams').then(r => r.data),
    api.get('/routes').then(r => r.data),
    api.get('/servers').then(r => r.data),
    api.get('/merge-servers').then(r => r.data),
  ])
}
onMounted(load)

function openAdd() {
  Object.assign(form, { ...defaultForm, upstream_id: upstreams.value[0]?.id, route_id: routes.value[0]?.id })
  selectedServers.value = []
  editing.value = false; showForm.value = true
}
function openEdit(row) {
  Object.assign(form, row)
  try { selectedServers.value = JSON.parse(row.server_ids) } catch { selectedServers.value = [] }
  editing.value = true; editId.value = row.id; showForm.value = true
}

async function handleSubmit() {
  const { upstream, route, merge_server, created_at, updated_at, id, ...clean } = form
  const payload = { ...clean, server_ids: JSON.stringify(selectedServers.value) }
  if (editing.value) await api.put(`/rules/${editId.value}`, payload)
  else await api.post('/rules', payload)
  ElMessage.success('已保存'); showForm.value = false; load()
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确定删除规则「${row.name}」？`, '确认')
  await api.delete(`/rules/${row.id}`)
  ElMessage.success('已删除'); load()
}
</script>
