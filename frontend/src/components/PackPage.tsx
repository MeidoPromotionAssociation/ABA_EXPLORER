import React, {useCallback, useEffect, useState} from "react";
import {Alert, App as AntdApp, Button, Card, Form, Input, Result, Space, Typography} from "antd";
import {FolderOpenOutlined, InboxOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {useNavigate} from "react-router-dom";
import {
    AbaExplorerService,
    App as AppService,
} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import type {PackResult} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import {setContainerPath, setCtPath, setUnpackedDir, useWorkspace} from "../hooks/workspace";
import {appMessage as message, describeError} from "../utils/feedback";
import {baseName, dirName, joinPath} from "../utils/format";

const {Text, Paragraph} = Typography;

/**
 * PackPage 把纯资源目录打回 ABA
 * MeidoSerialization 的 PackToAbaAndCt 固定输出 Unity 2022.3.35f1 的 ABA 与配套 CT，
 * 两个文件都写到所选目录的父目录，基名由输入决定
 */
const PackPage: React.FC = () => {
    const {t} = useTranslation();
    const navigate = useNavigate();
    const {modal} = AntdApp.useApp();
    const {unpackedDir} = useWorkspace();
    const [dirPath, setDirPath] = useState(unpackedDir);
    const [outputName, setOutputName] = useState("");
    const [packing, setPacking] = useState(false);
    const [result, setResult] = useState<PackResult | null>(null);

    // 工作区目录变化时同步表单，并按目录名推导默认基名
    const syncSuggestedName = useCallback(async (dir: string) => {
        if (!dir) {
            setOutputName("");
            return;
        }
        try {
            setOutputName(await AbaExplorerService.DefaultPackName(dir));
        } catch (error) {
            console.warn("suggest pack name failed:", error);
            setOutputName(baseName(dir));
        }
    }, []);

    useEffect(() => {
        setDirPath(unpackedDir);
        void syncSuggestedName(unpackedDir);
    }, [unpackedDir, syncSuggestedName]);

    const chooseDir = async () => {
        try {
            const dir = await AppService.SelectDirectory(t("PackPage.choose_dir"));
            if (!dir) return;
            setDirPath(dir);
            setResult(null);
            await syncSuggestedName(dir);
        } catch (error) {
            message.error(describeError(error));
        }
    };

    /** confirmOverwrite 目标文件已存在时询问，两个输出中任一存在都要提示 */
    const confirmOverwrite = async (paths: string[]): Promise<boolean> => {
        const existing: string[] = [];
        for (const path of paths) {
            const info = await AppService.StatPath(path);
            if (info.exists) existing.push(path);
        }
        if (existing.length === 0) return true;
        return new Promise((resolve) => {
            modal.confirm({
                title: t("PackPage.outputs_exist"),
                content: (
                    <div>
                        {existing.map((path) => (
                            <Paragraph key={path} style={{marginBottom: 4, fontSize: 12}}>{path}</Paragraph>
                        ))}
                    </div>
                ),
                okText: t("Common.overwrite"),
                okButtonProps: {danger: true},
                cancelText: t("Common.cancel"),
                onOk: () => resolve(true),
                onCancel: () => resolve(false),
            });
        });
    };

    const pack = async () => {
        const name = outputName.trim();
        if (!dirPath || !name) {
            message.warning(t("PackPage.missing_input"));
            return;
        }
        setPacking(true);
        try {
            const parent = dirName(dirPath);
            const abaTarget = joinPath(parent, `${name}.aba`);
            const ctTarget = joinPath(parent, `${name}.ct`);
            if (!(await confirmOverwrite([abaTarget, ctTarget]))) return;

            const packed = await AbaExplorerService.Pack(dirPath, name);
            if (!packed) return;
            setResult(packed);
            message.success(t("PackPage.packed", {name: baseName(packed.abaPath)}));
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setPacking(false);
        }
    };

    const openResult = (which: "aba" | "ct") => {
        if (!result) return;
        if (which === "aba") {
            setContainerPath(result.abaPath);
            navigate("/container");
            return;
        }
        setCtPath(result.ctPath);
        navigate("/ct");
    };

    return (
        <Card
            style={{flex: 1, minWidth: 0, overflow: "auto"}}
            title={t("PackPage.title")}
        >
            <Alert
                type="info"
                showIcon
                style={{marginBottom: 20}}
                title={t("PackPage.hint_title")}
                description={t("PackPage.hint_body")}
            />

            <Form layout="vertical" style={{maxWidth: 720}}>
                <Form.Item label={t("PackPage.source_dir")} required>
                    <Space.Compact style={{width: "100%"}}>
                        <Input
                            value={dirPath}
                            placeholder={t("PackPage.source_dir_placeholder")}
                            onChange={(event) => {
                                setDirPath(event.target.value);
                                setResult(null);
                            }}
                        />
                        <Button icon={<FolderOpenOutlined/>} onClick={chooseDir}>
                            {t("Common.browse")}
                        </Button>
                    </Space.Compact>
                </Form.Item>

                <Form.Item
                    label={t("PackPage.output_name")}
                    required
                    extra={
                        dirPath && outputName
                            ? t("PackPage.output_preview", {
                                aba: joinPath(dirName(dirPath), `${outputName.trim()}.aba`),
                                ct: joinPath(dirName(dirPath), `${outputName.trim()}.ct`),
                            })
                            : t("PackPage.output_name_extra")
                    }
                >
                    <Input
                        value={outputName}
                        placeholder={t("PackPage.output_name_placeholder")}
                        onChange={(event) => {
                            setOutputName(event.target.value);
                            setResult(null);
                        }}
                    />
                </Form.Item>

                <Form.Item>
                    <Space>
                        <Button
                            type="primary"
                            icon={<InboxOutlined/>}
                            loading={packing}
                            disabled={!dirPath || !outputName.trim()}
                            onClick={() => void pack()}
                        >
                            {t("PackPage.pack")}
                        </Button>
                        {dirPath && (
                            <Button
                                onClick={() => {
                                    setUnpackedDir(dirPath);
                                    navigate("/unpacked");
                                }}
                            >
                                {t("PackPage.browse_source")}
                            </Button>
                        )}
                    </Space>
                </Form.Item>
            </Form>

            {result && (
                <>
                    {result.warnings.length > 0 && (
                        <Alert
                            type="warning"
                            showIcon
                            style={{marginTop: 8}}
                            title={t("PackPage.warnings")}
                            description={
                                <Space vertical size={8}>
                                    {result.warnings.map((warning, index) => (
                                        <Text key={index} style={{fontSize: 12, whiteSpace: "pre-wrap"}}>
                                            {warning}
                                        </Text>
                                    ))}
                                </Space>
                            }
                        />
                    )}
                    <Result
                        status="success"
                        title={t("PackPage.result_title")}
                        subTitle={
                            <Space vertical size={2}>
                                <Text className="mono" style={{fontSize: 12}}>{result.abaPath}</Text>
                                <Text className="mono" style={{fontSize: 12}}>{result.ctPath}</Text>
                            </Space>
                        }
                        extra={[
                            <Button key="aba" type="primary" onClick={() => openResult("aba")}>
                                {t("PackPage.open_aba")}
                            </Button>,
                            <Button key="ct" onClick={() => openResult("ct")}>
                                {t("PackPage.open_ct")}
                            </Button>,
                            <Button
                                key="reveal"
                                icon={<FolderOpenOutlined/>}
                                onClick={async () => {
                                    try {
                                        await AppService.Reveal(result.abaPath);
                                    } catch (error) {
                                        message.error(describeError(error));
                                    }
                                }}
                            >
                                {t("Common.reveal")}
                            </Button>,
                        ]}
                    />
                </>
            )}
        </Card>
    );
};

export default PackPage;
