import {t} from "i18next";
import {App as AppService} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import {appMessage as message, describeError} from "../utils/feedback";
import {baseName} from "../utils/format";

// 协议是否注册在应用生命周期内不会变，查一次缓存下来
// Whether the protocol is registered does not change while the app runs, so the answer is cached after one query
let availabilityProbe: Promise<boolean> | null = null;

/** modEditorAvailable 返回系统是否注册过 KCES MOD EDITOR 的协议 */
export function modEditorAvailable(): Promise<boolean> {
    if (!availabilityProbe) {
        availabilityProbe = AppService.IsModEditorAvailable().catch((error) => {
            console.warn("probe KCES MOD EDITOR protocol failed:", error);
            return false;
        });
    }
    return availabilityProbe;
}

/**
 * openInModEditor 请求 KCES MOD EDITOR 打开一个文件
 * 未注册协议与格式不支持是两种不同情况，分别给出提示，避免用户点了按钮以为程序坏了
 */
export async function openInModEditor(path: string): Promise<void> {
    if (!(await modEditorAvailable())) {
        message.warning(t("ModEditor.not_installed"));
        return;
    }
    try {
        if (!(await AppService.CanOpenInModEditor(path))) {
            message.info(t("ModEditor.unsupported"));
            return;
        }
        await AppService.OpenInModEditor(path);
        message.success(t("ModEditor.opened", {name: baseName(path)}));
    } catch (error) {
        message.error(describeError(error));
    }
}
