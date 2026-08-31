import React, {useCallback, useEffect, useMemo, useState} from "react";
import {Alert, App as AntdApp, Button, Card, Dropdown, Skeleton, Space, Table, Tabs, Tag} from "antd";
import {CodeOutlined, DownOutlined, ExportOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {
    App as AppService,
    CtExplorerService,
} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import type {CtOverview} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import PathHeader from "./common/PathHeader";
import EmptyState from "./common/EmptyState";
import CatalogItemTable from "./ct/CatalogItemTable";
import CtOverviewPanel from "./ct/CtOverviewPanel";
import ExtensionLists from "./ct/ExtensionLists";
import VirtualFileTable from "./ct/VirtualFileTable";
import {setCtPath, useWorkspace} from "../hooks/workspace";
import useFileOpener from "../hooks/fileOpener";
import {CtTabKey} from "../utils/LocalStorageKeys";
import {CtFilter, JsonFilter} from "../utils/consts";
import {appMessage as message, describeError} from "../utils/feedback";
import {baseName, formatNumber} from "../utils/format";

/**
 * CtPage .ct 内容表浏览页
 * .ct 是 KCES 的资源目录容器，游戏靠其中的 catalog 虚拟文件索引 AssetBundle 里的资源
 * catalog 解码失败时仍展示虚拟文件表，便于排查损坏或非标准的内容表
 */
const CtPage: React.FC = () => {
    const {t} = useTranslation();
    const {modal} = AntdApp.useApp();
    const {ctPath} = useWorkspace();
    const {selectAndOpen} = useFileOpener();
    const [overview, setOverview] = useState<CtOverview | null>(null);
    const [loading, setLoading] = useState(false);
    const [busy, setBusy] = useState(false);
    const [tab, setTab] = useState(() => localStorage.getItem(CtTabKey) ?? "catalog");

    const load = useCallback(async () => {
        if (!ctPath) {
            setOverview(null);
            return;
        }
        setLoading(true);
        try {
            const result = await CtExplorerService.Inspect(ctPath);
            setOverview(result);
        } catch (error) {
            setOverview(null);
            message.error(describeError(error));
        } finally {
            setLoading(false);
        }
    }, [ctPath]);

    useEffect(() => {
        void load();
    }, [load]);

    const decodedNames = useMemo(() => {
        if (!overview) return [];
        return ["catalog", ...overview.extensions.map((list) => list.key)];
    }, [overview]);

    const extractAll = async () => {
        if (!ctPath) return;
        setBusy(true);
        try {
            const dir = await AppService.SelectDirectory(t("CtPage.choose_extract_dir"));
            if (!dir) return;
            const written = await CtExplorerService.ExtractAllVirtualFiles(ctPath, dir);
            message.success(t("CtPage.extracted_all", {count: written.length}));
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setBusy(false);
        }
    };

    const convertToJson = async () => {
        if (!ctPath) return;
        setBusy(true);
        try {
            const target = await AppService.SelectPathToSave(JsonFilter, t("Common.json_files"), baseName(ctPath) + ".json");
            if (!target) return;
            if (!(await confirmOverwrite(target))) return;
            await CtExplorerService.ConvertToJson(ctPath, target);
            message.success(t("CtPage.converted", {name: baseName(target)}));
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setBusy(false);
        }
    };

    /** confirmOverwrite 目标已存在时询问是否覆盖 */
    const confirmOverwrite = async (target: string): Promise<boolean> => {
        const info = await AppService.StatPath(target);
        if (!info.exists) return true;
        return new Promise((resolve) => {
            modal.confirm({
                title: t("Common.file_exists"),
                content: target,
                okText: t("Common.overwrite"),
                okButtonProps: {danger: true},
                cancelText: t("Common.cancel"),
                onOk: () => resolve(true),
                onCancel: () => resolve(false),
            });
        });
    };

    // 从编辑 JSON 写回 .ct：输出到当前文件时转换完直接重载，否则切到新文件
    // Writing a .ct back from editing JSON reloads in place when it targets the current file and switches to the new file otherwise
    const convertFromJson = async () => {
        setBusy(true);
        try {
            const source = await AppService.SelectFile(JsonFilter, t("Common.json_files"));
            if (!source) return;
            const target = await AppService.SelectPathToSave(CtFilter, t("Common.ct_files"), baseName(source).replace(/\.json$/i, ""));
            if (!target) return;
            if (!(await confirmOverwrite(target))) return;
            await CtExplorerService.ConvertFromJson(source, target);
            message.success(t("CtPage.imported", {name: baseName(target)}));
            if (target === ctPath) {
                await load();
            } else {
                setCtPath(target);
            }
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setBusy(false);
        }
    };

    if (!ctPath) {
        return (
            <EmptyState
                description={t("CtPage.no_file")}
                hint={t("Common.drop_hint")}
                actionLabel={t("NavBar.open_file")}
                onAction={selectAndOpen}
            />
        );
    }

    const catalog = overview?.catalog ?? null;

    return (
        <Card
            style={{flex: 1, minWidth: 0, display: "flex", flexDirection: "column"}}
            styles={{body: {flex: 1, minHeight: 0, display: "flex", flexDirection: "column", padding: 16}}}
        >
            <PathHeader
                path={ctPath}
                onReload={load}
                actions={
                    <>
                        <Button icon={<ExportOutlined/>} loading={busy} onClick={() => void extractAll()}>
                            {t("CtPage.extract_all")}
                        </Button>
                        <Dropdown.Button
                            loading={busy}
                            icon={<DownOutlined/>}
                            onClick={() => void convertToJson()}
                            menu={{
                                items: [{key: "import", label: t("CtPage.from_json")}],
                                onClick: () => void convertFromJson(),
                            }}
                        >
                            <CodeOutlined/> {t("CtPage.to_json")}
                        </Dropdown.Button>
                    </>
                }
            />

            {loading && !overview ? (
                <Skeleton active paragraph={{rows: 6}}/>
            ) : !overview ? (
                <EmptyState description={t("CtPage.load_failed")} actionLabel={t("Common.reload")} onAction={load}/>
            ) : (
                <>
                    {overview.catalogError && (
                        <Alert
                            type="warning"
                            showIcon
                            style={{marginBottom: 12}}
                            title={t("CtPage.catalog_error")}
                            description={overview.catalogError}
                        />
                    )}

                    <CtOverviewPanel overview={overview}/>

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
                            localStorage.setItem(CtTabKey, key);
                        }}
                        items={[
                            {
                                key: "catalog",
                                label: (
                                    <Space size={6}>
                                        {t("CtPage.tab_items")}
                                        <Tag style={{marginInlineEnd: 0}}>
                                            {formatNumber(catalog?.items.length ?? 0)}
                                        </Tag>
                                    </Space>
                                ),
                                children: catalog ? (
                                    <CatalogItemTable
                                        items={catalog.items}
                                        resourceFileNames={catalog.resourceFileNames}
                                        virtual={catalog.kind === "virtualAsset"}
                                    />
                                ) : (
                                    <EmptyState description={t("CtPage.no_catalog")}/>
                                ),
                            },
                            {
                                key: "extensions",
                                label: (
                                    <Space size={6}>
                                        {t("CtPage.tab_extensions")}
                                        <Tag style={{marginInlineEnd: 0}}>{formatNumber(overview.extensions.length)}</Tag>
                                    </Space>
                                ),
                                children: <ExtensionLists lists={overview.extensions}/>,
                            },
                            {
                                key: "files",
                                label: (
                                    <Space size={6}>
                                        {t("CtPage.tab_files")}
                                        <Tag style={{marginInlineEnd: 0}}>{formatNumber(overview.files.length)}</Tag>
                                    </Space>
                                ),
                                children: (
                                    <VirtualFileTable ctPath={ctPath} files={overview.files} decodedNames={decodedNames}/>
                                ),
                            },
                            {
                                key: "directories",
                                label: (
                                    <Space size={6}>
                                        {t("CtPage.tab_directories")}
                                        <Tag style={{marginInlineEnd: 0}}>
                                            {formatNumber(overview.directories.length)}
                                        </Tag>
                                    </Space>
                                ),
                                children: (
                                    <div style={{height: "100%", overflow: "auto"}}>
                                        <Table
                                            size="small"
                                            pagination={false}
                                            rowKey={(row) => row.path}
                                            dataSource={overview.directories}
                                            locale={{emptyText: t("CtPage.no_directories")}}
                                            columns={[
                                                {
                                                    title: t("CtPage.directory_path"),
                                                    dataIndex: "path",
                                                    defaultSortOrder: "ascend",
                                                    sorter: (left, right) =>
                                                        left.path.localeCompare(right.path, undefined, {numeric: true}),
                                                },
                                                {
                                                    title: t("CtPage.directory_version"),
                                                    dataIndex: "version",
                                                    width: 120,
                                                    sorter: (left, right) => left.version - right.version,
                                                },
                                            ]}
                                        />
                                    </div>
                                ),
                            },
                        ]}
                    />
                </>
            )}
        </Card>
    );
};

export default CtPage;
