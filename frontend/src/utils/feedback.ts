// 全局 message 桥：antd 静态 message API 无法消费 ConfigProvider 上下文，
// 深色模式与自定义主题色不会作用于其弹出提示。
// 由 App.tsx 组件树内的 MessageBinder 绑定 App.useApp() 实例，
// 供组件与非组件模块（hooks、utils）统一调用；绑定前回退到静态 API。
import {message as staticMessage} from "antd";
import type {MessageInstance} from "antd/es/message/interface";

let bound: MessageInstance | null = null;

/** bindMessage 绑定组件树内的 message 实例（由 App.tsx 挂载时调用） */
export function bindMessage(api: MessageInstance): void {
    bound = api;
}

/** appMessage 全局可用的 message：优先使用绑定实例，未绑定时回退静态 API */
export const appMessage: MessageInstance = new Proxy(staticMessage as unknown as MessageInstance, {
    get(target, prop) {
        return ((bound ?? target) as any)[prop];
    },
});

/** describeError 把后端错误统一整理成可展示的一行文本 */
export function describeError(error: unknown): string {
    if (error instanceof Error) return error.message;
    if (typeof error === "string") return error;
    try {
        return JSON.stringify(error);
    } catch {
        return String(error);
    }
}
