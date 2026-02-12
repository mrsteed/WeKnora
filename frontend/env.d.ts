/// <reference types="vite/client" />
// 配置这个文件是 解决错误：找不到模块“@/views/login/index.vue”或其相应的类型声明。ts(2307)
// 这段代码告诉 TypeScript，所有以 .vue 结尾的文件都是 Vue 组件，可以通过 import 语句进行导入。这样做通常可以解决无法识别模块的问题。
declare module '*.vue' {
    import { Component } from 'vue'; const component: Component; export default component;
}
// vue-i18n 全局类型增强，使 $t 等在模板中可用
import 'vue-i18n'

declare module 'vue' {
    interface ComponentCustomProperties {
        $t: import('vue-i18n').ComposerTranslation
        $d: import('vue-i18n').ComposerDateTimeFormatting
        $n: import('vue-i18n').ComposerNumberFormatting
        $tm: (key: string) => unknown
    }
}