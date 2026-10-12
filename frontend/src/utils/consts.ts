import {App as AppService} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";

// 应用版本由后端 internal/app.go 的 CurrentVersion 提供，顶层 await 让各处直接拿到字符串
// 取不到时兜底成 unknown：这个模块被几乎所有页面 import，抛错会让整个界面白屏
// The app version comes from CurrentVersion in internal/app.go, and a top-level await hands callers a plain string
// A failure falls back to unknown because almost every page imports this module and a throw here blanks the whole UI
export const AppVersion = await AppService.GetAppVersion().catch((error) => {
    console.error("read app version failed:", error);
    return "unknown";
});

export const UpdateCheckInterval = 24 * 60 * 60 * 1000; // 检查更新的间隔 24 小时（毫秒）

export const RetryInterval = 1 * 60 * 60 * 1000; // 重试检查更新间隔 1 小时（毫秒）

export const SettingCheckUpdateKey = "SettingCheckUpdateKey"; // 检查更新设置的键

// 后端推送的事件名，必须与 main.go 的 RegisterEvent 和 internal/protocol.go 的常量一字不差
// Event names pushed by the backend, which must match RegisterEvent in main.go and the constant in internal/protocol.go exactly
export const FileDroppedEvent = "explorer:file-dropped";
export const ProtocolOpenEvent = "explorer:protocol-open";

// 文件对话框过滤器：Wails 的 AddFilter 用分号分隔多个通配符
// File dialog filters: Wails AddFilter separates multiple wildcards with semicolons
export const ContainerFilter = "*.aba;*.asset_bg;*.asset_scene";
export const CtFilter = "*.ct";
export const AnyKcesFilter = "*.aba;*.asset_bg;*.asset_scene;*.ct";
export const JsonFilter = "*.json";

// 项目与依赖库的仓库地址，设置页用它们打开外部浏览器
// Repository URLs for this project and its serialization library, opened from the settings page
export const GitHubRepoUrl = "https://github.com/MeidoPromotionAssociation/ABA_EXPLORER";
export const GitHubReleaseUrl = "https://github.com/MeidoPromotionAssociation/ABA_EXPLORER/releases";
export const MeidoSerializationUrl = "https://github.com/MeidoPromotionAssociation/MeidoSerialization";
export const ModEditorUrl = "https://github.com/MeidoPromotionAssociation/KCES_MOD_EDITOR";

// MeidoSerialization 命令行文档按语言分节，同一个章节在不同语言里的标题不同，锚点也就各不相同；
// 韩语没有对应章节，回落到英语
// The MeidoSerialization CLI documentation is split by language, so a section carries a different anchor per
// language; Korean has no matching section and falls back to English
const MeidoSerializationCliDocBase = "https://github.com/MeidoPromotionAssociation/MeidoSerialization/blob/main/docs/cli-document.md";

const MeidoSerializationCliDocAnchors: Record<string, Record<string, string>> = {
    // KCES Model/Mesh/AnimationClip/AudioClip 一节
    gltf: {
        "en-US": "#kces-model-mesh-animationclip-and-audioclip",
        "zh-CN": "#kces-modelmeshanimationclip-%E4%B8%8E-audioclip",
        "ja-JP": "#kces-modelmeshanimationclipaudioclip",
    },
    // 修改 Sprite 一节
    sprite: {
        "en-US": "#editing-a-sprite",
        "zh-CN": "#%E4%BF%AE%E6%94%B9-sprite",
        "ja-JP": "#sprite-%E3%81%AE%E7%B7%A8%E9%9B%86",
    },
};

/** meidoSerializationCliDocUrl 按章节与界面语言返回文档链接，未知章节与未知语言都回落到英语 */
export function meidoSerializationCliDocUrl(section: string, language: string): string {
    const anchors = MeidoSerializationCliDocAnchors[section] ?? MeidoSerializationCliDocAnchors.gltf;
    return MeidoSerializationCliDocBase + (anchors[language] ?? anchors["en-US"]);
}

// 容器扩展名，用于在拖放与启动文件时判断该进入哪个页面
// Container extensions used to route dropped and startup files to the right page
export const ContainerExtensions = [".aba", ".asset_bg", ".asset_scene"];

/** isContainerPath 判断路径是否为 UnityFS 容器 */
export function isContainerPath(path: string): boolean {
    const lower = path.toLowerCase();
    return ContainerExtensions.some((ext) => lower.endsWith(ext));
}

/** isCtPath 判断路径是否为 .ct 内容表 */
export function isCtPath(path: string): boolean {
    return path.toLowerCase().endsWith(".ct");
}

// Unity 对象类型的 Tag 配色，键为 MeidoSerialization 报告的类型名
// Tag colors for Unity object types keyed by the type name MeidoSerialization reports
export const TypeColors: Record<string, string> = {
    AssetBundle: "purple",
    TextAsset: "blue",
    Texture2D: "magenta",
    Sprite: "volcano",
    SpriteAtlas: "orange",
    Mesh: "geekblue",
    AnimationClip: "cyan",
    Material: "gold",
    AudioClip: "lime",
    MonoBehaviour: "green",
    GameObject: "red",
};

/** typeColor 返回类型名对应的 Tag 配色，未知类型用默认灰色 */
export function typeColor(typeName: string): string | undefined {
    return TypeColors[typeName];
}

// 转换目标的展示顺序，Convert 服务返回的目标按此排序
// 3D 资源只提供 glb（展示为 glTF(glb)），不再单列 glTF
// Display order for conversion targets returned by the Convert service
// 3D assets only offer glb (shown as glTF(glb)), so glTF is no longer listed on its own
export const TargetOrder = ["json", "png", "glb", "audio", "csv"];

/** sortTargets 按展示顺序排列转换目标键 */
export function sortTargets<T extends { key: string }>(targets: T[]): T[] {
    return [...targets].sort((left, right) => {
        const leftIndex = TargetOrder.indexOf(left.key);
        const rightIndex = TargetOrder.indexOf(right.key);
        return (leftIndex < 0 ? TargetOrder.length : leftIndex) - (rightIndex < 0 ? TargetOrder.length : rightIndex);
    });
}
