import React, {useCallback, useEffect, useState} from "react";
import {App as AntdApp, Button, Card, Dropdown, Skeleton, Space, Tabs, Tag} from "antd";
import {DownOutlined, ExportOutlined, FileAddOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {useNavigate} from "react-router-dom";
import {
    AbaExplorerService,
    App as AppService,
    CtExplorerService,
} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import type {AbaOverview} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import PathHeader from "./common/PathHeader";
import EmptyState from "./common/EmptyState";
import AssetTable from "./container/AssetTable";
import ContainerOverview from "./container/ContainerOverview";
import {BlockTable, DirectoryTable} from "./container/StructureTables";
import SerializedFileList from "./container/SerializedFileList";
import {setCtPath, setUnpackedDir, useWorkspace} from "../hooks/workspace";
import useFileOpener from "../hooks/fileOpener";
import {AbaTabKey} from "../utils/LocalStorageKeys";
import {appMessage as message, describeError} from "../utils/feedback";
import {baseName, formatNumber} from "../utils/format";

/**
 * ContainerPage UnityFS 容器（.aba/.asset_bg/.asset_scene）浏览页
 * 概览展示 UnityFS 头部与块目录元数据，标签页分别是对象、数据块、目录条目和 SerializedFile 详情
 * 解包与生成 .ct 都会写磁盘，目标已存在时先让用户确认
 */
const ContainerPage: React.FC = () => {
    const {t} = useTranslation();
    const navigate = useNavigate();
    const {modal} = AntdApp.useApp();
    const {containerPath} = useWorkspace();
    const {selectAndOpen} = useFileOpener();
    const [overview, setOverview] = useState<AbaOverview | null>(null);
    const [loading, setLoading] = useState(false);
    const [busy, setBusy] = useState(false);
    const [tab, setTab] = useState(() => localStorage.getItem(AbaTabKey) ?? "assets");

    const load = useCallback(async () => {
        if (!containerPath) {
            setOverview(null);
            return;
        }
        setLoading(true);
        try {
            const result = await AbaExplorerService.Inspect(containerPath);
            setOverview(result);
        } catch (error) {
            setOverview(null);
            message.error(describeError(error));
        } finally {
            setLoading(false);
        }
    }, [containerPath]);

    useEffect(() => {
        void load();
    }, [load]);

    /** confirmOverwrite 目标已存在时询问是否覆盖，不存在时直接放行 */
    const confirmOverwrite = async (path: string, title: string): Promise<boolean> => {
        const info = await AppService.StatPath(path);
        if (!info.exists) return true;
        return new Promise((resolve) => {
            modal.confirm({
                title,
                content: path,
                okText: t("Common.overwrite"),
                okButtonProps: {danger: true},
                cancelText: t("Common.cancel"),
                onOk: () => resolve(true),
                onCancel: () => resolve(false),
            });
        });
    };

    const unpack = async (chooseDir: boolean) => {
        if (!containerPath) return;
        setBusy(true);
        try {
            let outDir = await AbaExplorerService.DefaultUnpackDir(containerPath);
            if (chooseDir) {
                const picked = await AppService.SelectDirectory(t("ContainerPage.choose_unpack_dir"));
                if (!picked) return;
                outDir = picked;
            } else if (!(await confirmOverwrite(outDir, t("ContainerPage.unpack_dir_exists")))) {
                return;
            }
            const result = await AbaExplorerService.Unpack(containerPath, outDir);
            if (!result) return;
            setUnpackedDir(result.outDir);
            message.success(t("ContainerPage.unpack_done", {count: result.files.length}));
            navigate("/unpacked");
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setBusy(false);
        }
    };

    const generateCt = async () => {
        if (!containerPath) return;
        setBusy(true);
        try {
            const target = containerPath.replace(/\.[^.\\/]+$/, "") + ".ct";
            if (!(await confirmOverwrite(target, t("ContainerPage.ct_exists")))) return;
            const written = await CtExplorerService.GenerateFromAba(containerPath, "");
            setCtPath(written);
            message.success(t("ContainerPage.ct_generated", {name: baseName(written)}));
            navigate("/ct");
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setBusy(false);
        }
    };

    if (!containerPath) {
        return (
            <EmptyState
                description={t("ContainerPage.no_file")}
                hint={t("Common.drop_hint")}
                actionLabel={t("NavBar.open_file")}
                onAction={selectAndOpen}
            />
        );
    }

    return (
        <Card
            style={{flex: 1, minWidth: 0, display: "flex", flexDirection: "column"}}
            styles={{body: {flex: 1, minHeight: 0, display: "flex", flexDirection: "column", padding: 16}}}
        >
            <PathHeader
                path={containerPath}
                onReload={load}
                actions={
                    <>
                        <Dropdown.Button
                            type="primary"
                            loading={busy}
                            icon={<DownOutlined/>}
                            onClick={() => void unpack(false)}
                            menu={{
                                items: [{key: "choose", label: t("ContainerPage.unpack_to")}],
                                onClick: () => void unpack(true),
                            }}
                        >
                            <ExportOutlined/> {t("ContainerPage.unpack")}
                        </Dropdown.Button>
                        <Button icon={<FileAddOutlined/>} loading={busy} onClick={() => void generateCt()}>
                            {t("ContainerPage.generate_ct")}
                        </Button>
                    </>
                }
            />

            {loading && !overview ? (
                <Skeleton active paragraph={{rows: 6}}/>
            ) : !overview ? (
                <EmptyState description={t("ContainerPage.load_failed")} actionLabel={t("Common.reload")} onAction={load}/>
            ) : (
                <>
                    <ContainerOverview overview={overview}/>

                    <Tabs
                        style={{flex: 1, minHeight: 0}}
                        className="fill-tabs"
                        styles={{
                            body: {flex: 1, minHeight: 0, display: "flex"},
                            content: {width: "100%", height: "100%", minHeight: 0},
                        }}
                        activeKey={tab}
                        onChange={(key) => {
                            setTab(key);
                            localStorage.setItem(AbaTabKey, key);
                        }}
                        items={[
                            {
                                key: "assets",
                                label: (
                                    <Space size={6}>
                                        {t("ContainerPage.tab_assets")}
                                        <Tag style={{marginInlineEnd: 0}}>{formatNumber(overview.assetCount)}</Tag>
                                    </Space>
                                ),
                                children: <AssetTable containerPath={containerPath} serializedFiles={overview.serializedFiles}/>,
                            },
                            {
                                key: "blocks",
                                label: (
                                    <Space size={6}>
                                        {t("ContainerPage.tab_blocks")}
                                        <Tag style={{marginInlineEnd: 0}}>{formatNumber(overview.blocks.length)}</Tag>
                                    </Space>
                                ),
                                children: <BlockTable blocks={overview.blocks}/>,
                            },
                            {
                                key: "directories",
                                label: (
                                    <Space size={6}>
                                        {t("ContainerPage.tab_directories")}
                                        <Tag style={{marginInlineEnd: 0}}>{formatNumber(overview.directories.length)}</Tag>
                                    </Space>
                                ),
                                children: <DirectoryTable directories={overview.directories}/>,
                            },
                            {
                                key: "serialized",
                                label: (
                                    <Space size={6}>
                                        {t("ContainerPage.tab_serialized")}
                                        <Tag style={{marginInlineEnd: 0}}>
                                            {formatNumber(overview.serializedFiles.length)}
                                        </Tag>
                                    </Space>
                                ),
                                children: <SerializedFileList files={overview.serializedFiles}/>,
                            },
                        ]}
                    />
                </>
            )}
        </Card>
    );
};

export default ContainerPage;
