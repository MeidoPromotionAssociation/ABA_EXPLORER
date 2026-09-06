import {useEffect, useRef, useState} from "react";

/**
 * 导航栏"打开"按钮的当前含义
 *
 * 各页面要打开的东西并不一样：容器页要选 .aba，内容表页要选 .ct，
 * 解包产物页与打包页要选目录，搜索页要选索引根目录。
 * 页面在挂载期间把自己的打开动作登记进来，导航栏渲染登记的那一个，
 * 没有页面登记时回落到"打开文件"这个通用入口。
 *
 * 与 workspace 相同的模块级存储 + 订阅模式，页面与导航栏是兄弟节点，
 * 走 context 要把 Provider 提到 Layout 之外，收益不值得
 */
export interface OpenAction {
    /** 按钮文案，同时用于快捷键提示 */
    label: string;
    /** 点击或按下 Ctrl+O 时执行 */
    run: () => void | Promise<void>;
}

let current: OpenAction | null = null;
const listeners = new Set<() => void>();

function notify(): void {
    listeners.forEach((listener) => listener());
}

/**
 * useProvideOpenAction 在页面挂载期间登记该页的打开动作
 *
 * run 每次渲染都会更新到 ref，因此调用方不必为它维持稳定的身份；
 * 只有 label 变化（切换界面语言）才会重新登记并让导航栏重渲染
 */
export function useProvideOpenAction(label: string, run: () => void | Promise<void>): void {
    const latestRun = useRef(run);
    latestRun.current = run;

    useEffect(() => {
        const action: OpenAction = {label, run: () => latestRun.current()};
        current = action;
        notify();
        return () => {
            // 路由切换时 React 先跑旧页面的清理再跑新页面的登记，
            // 身份判断避免这个顺序反过来时把新页面刚登记的动作清掉
            if (current === action) {
                current = null;
                notify();
            }
        };
    }, [label]);
}

/** useOpenAction 订阅当前登记的打开动作，无人登记时返回 null */
export function useOpenAction(): OpenAction | null {
    const [action, setAction] = useState(current);

    useEffect(() => {
        const listener = () => setAction(current);
        listeners.add(listener);
        // 订阅建立前可能已经有页面登记过，补读一次避免错过
        listener();
        return () => {
            listeners.delete(listener);
        };
    }, []);

    return action;
}
