import React from "react";
import {Button, Space, Tooltip, Typography, theme} from "antd";
import {FolderOpenOutlined, ReloadOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {App as AppService} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import {appMessage as message, describeError} from "../../utils/feedback";
import {baseName, dirName} from "../../utils/format";

const {Text} = Typography;

interface PathHeaderProps {
    path: string;
    /** 右侧操作区 */
    actions?: React.ReactNode;
    onReload?: () => void;
}

/**
 * PathHeader 当前文件的标题栏
 * 文件名与所在目录分两行，避免长路径把操作按钮挤出视口
 */
const PathHeader: React.FC<PathHeaderProps> = ({path, actions, onReload}) => {
    const {t} = useTranslation();
    const {token} = theme.useToken();

    const reveal = async () => {
        try {
            await AppService.Reveal(path);
        } catch (error) {
            message.error(describeError(error));
        }
    };

    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: 12,
                paddingBottom: 12,
                marginBottom: 12,
                borderBottom: `1px solid ${token.colorBorderSecondary}`,
            }}
        >
            <div style={{flex: 1, minWidth: 0}}>
                <Text strong ellipsis style={{display: "block", fontSize: token.fontSizeLG}}>
                    {baseName(path)}
                </Text>
                <Text type="secondary" ellipsis style={{display: "block", fontSize: 12}}>
                    {dirName(path)}
                </Text>
            </div>
            <Space style={{flexShrink: 0}}>
                {actions}
                <Tooltip title={t("Common.reveal")}>
                    <Button icon={<FolderOpenOutlined/>} onClick={reveal}/>
                </Tooltip>
                {onReload && (
                    <Tooltip title={t("Common.reload")}>
                        <Button icon={<ReloadOutlined/>} onClick={onReload}/>
                    </Tooltip>
                )}
            </Space>
        </div>
    );
};

export default PathHeader;
