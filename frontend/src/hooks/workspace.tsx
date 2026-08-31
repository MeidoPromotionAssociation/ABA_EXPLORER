import {useEffect, useState} from "react";
import {RecentFilesKey, RecentFilesLimit} from "../utils/LocalStorageKeys";

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

let current: WorkspaceState = {containerPath: "", ctPath: "", unpackedDir: ""};
const listeners = new Set<() => void>();

function notify(): void {
    listeners.forEach((listener) => listener());
}

/** setContainerPath 记录当前容器路径 */
export function setContainerPath(path: string): void {
    current = {...current, containerPath: path};
    rememberRecentFile(path);
    notify();
}

/** setCtPath 记录当前内容表路径 */
export function setCtPath(path: string): void {
    current = {...current, ctPath: path};
    rememberRecentFile(path);
    notify();
}

/** setUnpackedDir 记录当前解包目录 */
export function setUnpackedDir(dir: string): void {
    current = {...current, unpackedDir: dir};
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

/** readRecentFiles 读取最近打开的文件列表，内容损坏时返回空列表 */
export function readRecentFiles(): string[] {
    try {
        const saved = JSON.parse(localStorage.getItem(RecentFilesKey) ?? "[]");
        return Array.isArray(saved) ? saved.filter((item) => typeof item === "string") : [];
    } catch {
        return [];
    }
}

/** rememberRecentFile 把路径挪到最近列表首位并裁剪到上限 */
export function rememberRecentFile(path: string): void {
    if (!path) return;
    const existing = readRecentFiles().filter((item) => item !== path);
    localStorage.setItem(RecentFilesKey, JSON.stringify([path, ...existing].slice(0, RecentFilesLimit)));
}

/** clearRecentFiles 清空最近打开列表 */
export function clearRecentFiles(): void {
    localStorage.removeItem(RecentFilesKey);
}
