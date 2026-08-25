<template>
  <el-config-provider :locale="elementLocale">
    <router-view />
  </el-config-provider>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import en from 'element-plus/es/locale/lang/en'
import { currentLocale } from './i18n/locale'

const { locale } = useI18n()
// App 为根组件，永远取当前 i18n 语言；Element Plus 的 locale 随之切换。
locale.value = currentLocale()
const elementLocale = computed(() => (locale.value === 'zh-CN' ? zhCn : en))
</script>
