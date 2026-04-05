<template>
  <div>
    <div style="display:flex;justify-content:space-between;margin-bottom:16px">
      <h3 style="margin:0">路由管理 (UUID → 回源目标)</h3>
      <div style="display:flex;gap:8px">
        <el-button type="warning" @click="handleSyncAll">全部同步到服务器</el-button>
        <el-button type="primary" @click="openAdd">添加路由</el-button>
      </div>
    </div>
    <el-table :data="items" border stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column label="上游面板" width="150">
        <template #default="{row}">{{ row.upstream?.name || row.upstream_id }}</template>
      </el-table-column>
      <el-table-column prop="uuid" label="UUID" width="180">
        <template #default="{row}"><code style="font-size:12px">{{ row.uuid }}</code></template>
      </el-table-column>
      <el-table-column prop="forward" label="回源目标" width="200" />
      <el-table-column prop="remark" label="备注" min-width="150" />
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

    <el-dialog v-model="showForm" :title="editing?'编辑路由':'添加路由'" width="500px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="上游面板">
          <el-select v-model="form.upstream_id" style="width:100%">
            <el-option v-for="u in upstreams" :key="u.id" :label="u.name" :value="u.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="回源目标"><el-input v-model="form.forward" placeholder="1.2.3.4:30301" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.remark" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item label="UUID" v-if="editing"><el-input v-model="form.uuid" disabled /></el-form-item>
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
const showForm = ref(false)
const editing = ref(false)
const editId = ref(null)
const form = reactive({ upstream_id: null, forward: '', remark: '', enabled: true, uuid: '' })

async function load() {
  items.value = (await api.get('/routes')).data
  upstreams.value = (await api.get('/upstreams')).data
}
onMounted(load)

function openAdd() { Object.assign(form, { upstream_id: upstreams.value[0]?.id, forward: '', remark: '', enabled: true, uuid: '' }); editing.value = false; showForm.value = true }
function openEdit(row) { Object.assign(form, row); editing.value = true; editId.value = row.id; showForm.value = true }

async function handleSubmit() {
  if (editing.value) await api.put(`/routes/${editId.value}`, form)
  else await api.post('/routes', form)
  ElMessage.success('已保存'); showForm.value = false; load()
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确定删除路由「${row.uuid}」？`, '确认')
  await api.delete(`/routes/${row.id}`)
  ElMessage.success('已删除'); load()
}

async function handleSyncAll() {
  const { data } = await api.post('/routes/sync-all')
  const msgs = Object.entries(data).map(([k, v]) => `${k}: ${v}`).join('\n')
  ElMessage({ message: msgs || '没有需要同步的服务器', type: 'success', duration: 5000 })
}
</script>
