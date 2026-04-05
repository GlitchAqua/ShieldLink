<template>
  <el-container style="height:100vh">
    <el-aside width="200px" style="background:#304156">
      <div style="color:#fff;text-align:center;padding:20px 0;font-size:16px;font-weight:bold">ShieldLink</div>
      <el-menu :default-active="$route.path" router background-color="#304156" text-color="#bfcbd9" active-text-color="#409EFF">
        <el-menu-item index="/dashboard"><el-icon><Odometer /></el-icon><span>仪表盘</span></el-menu-item>
        <el-menu-item index="/upstreams"><el-icon><Connection /></el-icon><span>上游面板</span></el-menu-item>
        <el-menu-item index="/servers"><el-icon><Monitor /></el-icon><span>服务器管理</span></el-menu-item>
        <el-menu-item index="/routes"><el-icon><Guide /></el-icon><span>路由管理</span></el-menu-item>
        <el-menu-item index="/rules"><el-icon><SetUp /></el-icon><span>装饰规则</span></el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header style="display:flex;justify-content:flex-end;align-items:center;border-bottom:1px solid #eee">
        <span style="margin-right:12px">{{ auth.user?.username }}</span>
        <el-button size="small" @click="handleLogout">退出</el-button>
      </el-header>
      <el-main style="background:#f5f7fa">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()

function handleLogout() {
  auth.logout()
  router.push('/login')
}
</script>
