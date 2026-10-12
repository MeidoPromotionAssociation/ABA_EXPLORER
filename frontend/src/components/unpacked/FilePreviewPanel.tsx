import React, {useCallback, useEffect, useState} from "react";
import {Alert, Button, Descriptions, Divider, Empty, Space, Spin, Tag, theme, Tooltip, Typography} from "antd";
import {EditOutlined, FolderOpenOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {Browser} from "@wailsio/runtime";
import {
    App as AppService,
    ConvertService,
} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import type {
    ConvertTarget,
    FilePreview,
    UnpackedFile,
} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import {meidoSerializationCliDocUrl, sortTargets, typeColor} from "../../utils/consts";
import {resolveUiLanguage} from "../../utils/i18n";
import {openInModEditor} from "../../hooks/modEditor";
import {appMessage as message, describeError} from "../../utils/feedback";
import {baseName, formatBytes} from "../../utils/format";

const {Text, Paragraph} = Typography;

/**
 * FilePreviewPanel 选中解包文件的详情、可用转换目标与内容预览
 * 转换目标由后端按文件真实内容判定，因此不同 Texture2D 与 TextAsset 会给出不同按钮
 */
const FilePreviewPanel: React.FC<{ file: UnpackedFile | null }> = ({file}) => {
    const {t} = useTranslation();
    const {token} = theme.useToken();
    const [targets, setTargets] = useState<ConvertTarget[]>([]);
    const [preview, setPreview] = useState<FilePreview | null>(null);
    const [imageUrl, setImageUrl] = useState("");
    const [imageError, setImageError] = useState("");
    const [canModEdit, setCanModEdit] = useState(false);
    const [loading, setLoading] = useState(false);
    const [converting, setConverting] = useState("");

    const load = useCallback(async () => {
        setImageUrl("");
        setImageError("");
        setCanModEdit(false);
        if (!file) {
            setTargets([]);
            setPreview(null);
            return;
        }
        setLoading(true);
        try {
            const [availableTargets, filePreview, editable] = await Promise.all([
                ConvertService.Targets(file.absPath),
                AppService.PreviewFile(file.absPath),
                AppService.CanOpenInModEditor(file.absPath),
            ]);
            const sorted = sortTargets(availableTargets ?? []);
            setTargets(sorted);
            setPreview(filePreview);
            setCanModEdit(editable);

            // 可导出 PNG 的文件就是原生 Texture2D 或 Sprite，解码后直接渲染缩略图，
            // 解码失败（例如超出预览上限）时保留下方的字节预览并说明原因
            // A file that can export PNG is a native Texture2D or Sprite, so it renders as a thumbnail,
            // and a failed decode (an oversized preview, for instance) keeps the byte preview below with the reason shown
            if (sorted.some((target) => target.key === "png")) {
                try {
                    setImageUrl(await ConvertService.PreviewImage(file.absPath));
                } catch (error) {
                    setImageError(describeError(error));
                }
            }
        } catch (error) {
            setPreview(null);
            message.error(describeError(error));
        } finally {
            setLoading(false);
        }
    }, [file]);

    useEffect(() => {
        void load();
    }, [load]);

    const convert = async (target: ConvertTarget) => {
        if (!file) return;
        setConverting(target.key);
        try {
            const written = await ConvertService.Convert(file.absPath, target.key, "");
            message.success(t("FilePreviewPanel.converted", {name: baseName(written)}));
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setConverting("");
        }
    };

    // 在系统文件管理器里定位这个文件，转换出的产物默认就写在它旁边
    // Locating the file in the system file manager, where conversions land next to it by default
    const reveal = async () => {
        if (!file) return;
        try {
            await AppService.Reveal(file.absPath);
        } catch (error) {
            message.error(describeError(error));
        }
    };

    if (!file) {
        return (
            <div style={{height: "100%", display: "flex", alignItems: "center", justifyContent: "center"}}>
                <Empty description={t("FilePreviewPanel.no_selection")} image={Empty.PRESENTED_IMAGE_SIMPLE}/>
            </div>
        );
    }

    // 独立 Mesh 只含几何体，直接转 glb 得到的文件不完整（缺骨架与层级），
    // 需要完整模型时应当改转对应的 Model 文件，这里在信息区给出提示
    // A standalone Mesh carries geometry only, so a direct glb export is incomplete,
    // and the matching Model file is the right input for a full model, so the info area warns about it
    const meshGltfHint = file.kind === "Mesh" && targets.some((target) => target.key === "glb");

    // 提示浮层里的文档链接按界面语言指向对应章节，各语言的锚点不同
    // The docs link inside a tooltip points at the matching section for the current UI language, whose anchor differs per language
    const docLink = (section: string) => (
        <a
            style={{color: "inherit", textDecoration: "underline", cursor: "pointer"}}
            onClick={() => Browser.OpenURL(meidoSerializationCliDocUrl(section, resolveUiLanguage()))}
        >
            {t("FilePreviewPanel.doc_link")}
        </a>
    );
    const tooltipBodyStyle: React.CSSProperties = {display: "flex", flexDirection: "column", gap: 4};

    // glb 按钮：应当转换 .model（会带上它引用的 .mmesh），以及在 Blender 里使用 glb 的注意事项
    // The glb button: convert the .model (which pulls in the .mmesh it references), plus the Blender caveats for glb
    const gltfTooltip = (
        <div style={tooltipBodyStyle}>
            <span>{t("FilePreviewPanel.gltf_tooltip")}</span>
            <span>{t("FilePreviewPanel.blender_tooltip")}</span>
            {docLink("gltf")}
        </div>
    );

    // PNG 按钮：Sprite 不存储像素，PNG 导出是单向的，改外观要编辑它引用的 Texture2D 再重新打包
    // The PNG button: a Sprite stores no pixels, so the PNG export is one-way and changing its look means editing the referenced Texture2D and repacking
    const spriteTooltip = (
        <div style={tooltipBodyStyle}>
            <span>{t("FilePreviewPanel.sprite_tooltip")}</span>
            {docLink("sprite")}
        </div>
    );

    return (
        <div style={{height: "100%", display: "flex", flexDirection: "column", minHeight: 0, gap: 12, paddingLeft: 12}}>
            <Descriptions
                size="small"
                column={1}
                items={[
                    {
                        key: "name",
                        label: t("FilePreviewPanel.name"),
                        children: <Text copyable>{baseName(file.relPath)}</Text>
                    },
                    {
                        key: "kind",
                        label: t("FilePreviewPanel.kind"),
                        children: file.kind ? <Tag color={typeColor(file.kind)}>{file.kind}</Tag> : "-",
                    },
                    {key: "size", label: t("FilePreviewPanel.size"), children: formatBytes(file.size)},
                    {
                        key: "relPath",
                        label: t("FilePreviewPanel.rel_path"),
                        children: <Text type="secondary">{file.relPath}</Text>
                    },
                ]}
            />

            {meshGltfHint && (
                <Alert
                    showIcon
                    description={t("FilePreviewPanel.mesh_hint_body")}
                />
            )}

            <Space wrap>
                <Text strong>{t("FilePreviewPanel.convert_to")}</Text>
                {targets.length === 0 ? (
                    <Text type="secondary">{t("FilePreviewPanel.no_targets")}</Text>
                ) : (
                    targets.map((target) => {
                        const button = (
                            <Button
                                size="small"
                                type="primary"
                                ghost
                                loading={converting === target.key}
                                onClick={() => void convert(target)}
                            >
                                {t(`Target.${target.key}`)}
                            </Button>
                        );
                        // 只有 Sprite 的 PNG 导出才附「修改 Sprite」的说明，Texture2D 的 PNG 没有这个限制
                        // Only a Sprite PNG export carries the "editing a Sprite" note, a Texture2D PNG has no such restriction
                        if (target.key === "png" && file.kind === "Sprite") {
                            return (
                                <Tooltip key={target.key} styles={{root: {maxWidth: 380}}} title={spriteTooltip}>
                                    {button}
                                </Tooltip>
                            );
                        }
                        if (target.key === "glb") {
                            return (
                                <Tooltip key={target.key} styles={{root: {maxWidth: 380}}} title={gltfTooltip}>
                                    {button}
                                </Tooltip>
                            );
                        }
                        return <React.Fragment key={target.key}>{button}</React.Fragment>;
                    })
                )}
                <Divider vertical style={{margin: 0}}/>
                {canModEdit && (
                    <Button
                        size="small"
                        icon={<EditOutlined/>}
                        onClick={() => void openInModEditor(file.absPath)}
                    >
                        {t("ModEditor.open_in_editor")}
                    </Button>
                )}
                <Button size="small" icon={<FolderOpenOutlined/>} onClick={() => void reveal()}>
                    {t("Common.reveal")}
                </Button>
            </Space>

            {imageError && (
                <Text type="warning" style={{fontSize: 12}}>
                    {t("FilePreviewPanel.image_failed", {reason: imageError})}
                </Text>
            )}

            <div
                style={{
                    flex: 1,
                    minHeight: 0,
                    overflow: "auto",
                    background: token.colorFillQuaternary,
                    borderRadius: token.borderRadius,
                    padding: 8,
                }}
            >
                {loading ? (
                    <Spin/>
                ) : imageUrl ? (
                    <div className="image-preview">
                        <img src={imageUrl} alt={baseName(file.relPath)}/>
                    </div>
                ) : !preview ? (
                    <Text type="secondary">{t("FilePreviewPanel.no_preview")}</Text>
                ) : preview.isText ? (
                    <pre className="text-preview">{preview.text}</pre>
                ) : (
                    <Text type="secondary">{t("FilePreviewPanel.binary_content")}</Text>
                )}
                {!imageUrl && preview?.isText && preview.truncated && (
                    <Paragraph type="secondary" style={{fontSize: 12, marginTop: 8, marginBottom: 0}}>
                        {t("FilePreviewPanel.truncated")}
                    </Paragraph>
                )}
            </div>
        </div>
    );
};

export default FilePreviewPanel;
