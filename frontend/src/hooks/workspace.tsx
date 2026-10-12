import {useEffect, useState} from "react";
import {RecentFilesKey, RecentFilesLimit} from "../utils/LocalStorageKeys";
import {isContainerPath, isCtPath} from "../utils/consts";

/**
 * 工作区状态：当前打开的容器、内容表与解包目录
 * 与 themeSwitch 相同的模块级存储 + 订阅模式，让导航栏和各页面共享同一份路径，
 * 而不必把 Windows 路径（含反斜杠与中文）编码进 URL
 */
export interface WorkspaceState {
    /** 当前 .aba/.asset_bg/.asset_scene 路径 */
    containerPath: string;
    /** 当前 .ct 路径 */
    ctPath: string;
    /** 当前解包目录 */
    unpackedDir: string;
}

/**
 * RecentKind 是最近打开条目的种类，决定首页列表上的图标与类型标签
 * 只用于展示：点击条目仍然交给 openPath 按内容判定去向，旧数据推断错了也不会打开错页面
 */
export type RecentKind = "container" | "ct" | "unpacked";

/** RecentEntry 是最近打开列表中的一项 */
export interface RecentEntry {
    /** 文件或目录的绝对路径 */
    path: string;
    /** 条目种类 */
    kind: RecentKind;
}

let current: WorkspaceState = {containerPath: "", ctPath: "", unpackedDir: ""};
const listeners = new Set<() => void>();

function notify(): void {
    listeners.forEach((listener) => listener());
}

/** setContainerPath 记录当前容器路径 */
export function setContainerPath(path: string): void {
    current = {...current, containerPath: path};
    rememberRecentFile(path, "container");
    notify();
}

/** setCtPath 记录当前内容表路径 */
export function setCtPath(path: string): void {
    current = {...current, ctPath: path};
    rememberRecentFile(path, "ct");
    notify();
}

/** setUnpackedDir 记录当前解包目录，解包产物同样进入最近打开列表 */
export function setUnpackedDir(dir: string): void {
    current = {...current, unpackedDir: dir};
    rememberRecentFile(dir, "unpacked");
    notify();
}

/** getWorkspace 返回当前工作区状态 */
export function getWorkspace(): WorkspaceState {
    return current;
}

/** useWorkspace 订阅工作区状态 */
export function useWorkspace(): WorkspaceState {
    const [state, setState] = useState(current);

    useEffect(() => {
        const listener = () => setState(current);
        listeners.add(listener);
        return () => {
            listeners.delete(listener);
        };
    }, []);

    return state;
}

/** isRecentKind 判断一个值是否是合法的条目种类 */
function isRecentKind(value: unknown): value is RecentKind {
    return value === "container" || value === "ct" || value === "unpacked";
}

/** inferRecentKind 为没有种类信息的旧条目按扩展名推断种类，推断不出时按目录处理 */
function inferRecentKind(path: string): RecentKind {
    if (isCtPath(path)) return "ct";
    if (isContainerPath(path)) return "container";
    return "unpacked";
}

/** normalizeRecentEntry 兼容只存路径字符串的旧格式与 {path, kind} 的新格式 */
function normalizeRecentEntry(raw: unknown): RecentEntry | null {
    if (typeof raw === "string") {
        return raw ? {path: raw, kind: inferRecentKind(raw)} : null;
    }
    if (raw && typeof raw === "object") {
        const {path, kind} = raw as {path?: unknown; kind?: unknown};
        if (typeof path !== "string" || !path) return null;
        return {path, kind: isRecentKind(kind) ? kind : inferRecentKind(path)};
    }
    return null;
}

/** readRecentFiles 读取最近打开列表，内容损坏时返回空列表 */
export function readRecentFiles(): RecentEntry[] {
    try {
        const saved = JSON.parse(localStorage.getItem(RecentFilesKey) ?? "[]");
        if (!Array.isArray(saved)) return [];
        return saved
            .map(normalizeRecentEntry)
            .filter((entry): entry is RecentEntry => entry !== null);
    } catch {
        return [];
    }
}

/** rememberRecentFile 把路径挪到最近列表首位并裁剪到上限 */
export function rememberRecentFile(path: string, kind: RecentKind): void {
    if (!path) return;
    const existing = readRecentFiles().filter((entry) => entry.path !== path);
    const next = [{path, kind}, ...existing].slice(0, RecentFilesLimit);
    localStorage.setItem(RecentFilesKey, JSON.stringify(next));
}

/** clearRecentFiles 清空最近打开列表 */
export function clearRecentFiles(): void {
    localStorage.removeItem(RecentFilesKey);
}
