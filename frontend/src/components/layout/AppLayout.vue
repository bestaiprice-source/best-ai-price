<template>
  <div class="min-h-screen bg-[#ededed] text-[#090c0a] font-sans antialiased flex flex-col">
    <!-- Top Header -->
    <AppHeader />

    <!-- Body Layout: Sidebar + Main Content -->
    <div class="flex-1 flex gap-2.5 px-2.5 pb-2.5 pt-0 min-h-0">
      <!-- Sidebar -->
      <AppSidebar />

      <!-- Main Content Area -->
      <main
        class="flex-1 min-w-0 rounded-2xl bg-white border border-black/[0.07] p-4 md:p-6 lg:p-7 shadow-[0_0_20px_rgba(0,0,0,0.03)] overflow-y-auto"
      >
        <div class="mx-auto max-w-[1600px]">
          <slot />
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const authStore = useAuthStore()
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
