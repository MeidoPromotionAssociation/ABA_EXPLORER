import {useCallback} from "react";
import {useNavigate} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {App as AppService} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import {setContainerPath, setCtPath, setUnpackedDir} from "./workspace";
import {AnyKcesFilter, ContainerFilter, CtFilter, isContainerPath, isCtPath} from "../utils/consts";
import {appMessage as message, describeError} from "../utils/feedback";

/**
 * useFileOpener 统一处理"拿到一个路径后去哪个页面"
 * 先用后端按内容判定类型，判定不出结果时回退到扩展名，
 * 因为解包产物和游戏文件都可能没有可识别的签名
 */
export function useFileOpener() {
    const navigate = useNavigate();
    const {t} = useTranslation();

    const openPath = useCallback(async (path: string): Promise<void> => {
        if (!path) return;
        let fileType = "";
        try {
            const info = await AppService.DetermineFileType(path);
            fileType = info?.FileType ?? "";
        } catch (error) {
            // 判定失败不阻塞打开，扩展名足以区分容器与内容表
            console.warn("determine file type failed:", error);
        }

        if (fileType === "aba" || fileType === "asset_bg" || fileType === "asset_scene" || isContainerPath(path)) {
            setContainerPath(path);
            navigate("/container");
            return;
        }
        if (fileType === "ct" || fileType === "virtualdirectory" || isCtPath(path)) {
            setCtPath(path);
            navigate("/ct");
            return;
        }

        // 目录被拖进来时按解包目录处理
        try {
            const info = await AppService.StatPath(path);
            if (info.isDir) {
                setUnpackedDir(path);
                navigate("/unpacked");
                return;
            }
        } catch (error) {
            message.error(describeError(error));
            return;
        }
        message.warning(t("Common.unsupported_file", {path}));
    }, [navigate, t]);

    /**
     * selectWithFilter 用指定过滤器选一个文件再交给 openPath
     * 仍然过 openPath 而不是直接写工作区：过滤器只是对话框的默认视图，
     * 用户手打文件名能绕过它，按内容判定后落到正确的页面比信任扩展名可靠
     */
    const selectWithFilter = useCallback(async (filter: string, label: string): Promise<void> => {
        try {
            const path = await AppService.SelectFile(filter, label);
            if (path) await openPath(path);
        } catch (error) {
            message.error(describeError(error));
        }
    }, [openPath]);

    const selectAndOpen = useCallback(
        () => selectWithFilter(AnyKcesFilter, t("Common.kces_files")),
        [selectWithFilter, t]
    );

    const selectContainer = useCallback(
        () => selectWithFilter(ContainerFilter, t("Common.container_files")),
        [selectWithFilter, t]
    );

    const selectCt = useCallback(
        () => selectWithFilter(CtFilter, t("Common.ct_files")),
        [selectWithFilter, t]
    );

    return {openPath, selectAndOpen, selectContainer, selectCt};
}

export default useFileOpener;
