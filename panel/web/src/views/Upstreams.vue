<template>
  <div>
    <div style="display:flex;justify-content:space-between;margin-bottom:16px">
      <h3 style="margin:0">上游面板管理</h3>
      <el-button type="primary" @click="openAdd">添加上游</el-button>
    </div>
    <el-table :data="items" border stripe>
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="name" label="名称" width="150" />
      <el-table-column prop="domain" label="订阅域名" width="200" />
      <el-table-column prop="url" label="面板地址" min-width="250" show-overflow-tooltip />
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

    <el-dialog v-model="showForm" :title="editing?'编辑上游':'添加上游'" width="500px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="订阅域名"><el-input v-model="form.domain" placeholder="sub.example.com" /></el-form-item>
        <el-form-item label="面板地址"><el-input v-model="form.url" placeholder="https://panel.example.com" /></el-form-item>
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
const showForm = ref(false)
const editing = ref(false)
const editId = ref(null)
const form = reactive({ name: '', domain: '', url: '', enabled: true })

async function load() { items.value = (await api.get('/upstreams')).data }
onMounted(load)

function openAdd() { Object.assign(form, { name: '', domain: '', url: '', enabled: true }); editing.value = false; showForm.value = true }
function openEdit(row) { Object.assign(form, row); editing.value = true; editId.value = row.id; showForm.value = true }

async function handleSubmit() {
  if (editing.value) await api.put(`/upstreams/${editId.value}`, form)
  else await api.post('/upstreams', form)
  ElMessage.success('已保存'); showForm.value = false; load()
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确定删除「${row.name}」？`, '确认')
  await api.delete(`/upstreams/${row.id}`)
  ElMessage.success('已删除'); load()
}
</script>
