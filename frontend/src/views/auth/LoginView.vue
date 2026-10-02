<template>
  <div class="flex min-h-screen items-center justify-center bg-gray-50 dark:bg-dark-900">
    <div class="flex flex-col items-center gap-3">
      <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600 border-t-transparent"></div>
      <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('common.loading') }}...</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

onMounted(async () => {
  await authStore.checkAuth()
  if (authStore.isAuthenticated) {
    const redirectPath = (route.query.redirect as string) || (authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
    router.replace(redirectPath)
    return
  }
  const redirectParam = route.query.redirect as string | undefined
  const target = redirectParam ? `/login?redirect=${encodeURIComponent(redirectParam)}` : '/login'
  window.location.href = target
})
</script>
