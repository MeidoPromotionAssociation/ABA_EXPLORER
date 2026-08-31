import React, {useCallback, useEffect, useMemo, useState} from "react";
import {
    Button,
    Card,
    Checkbox,
    Input,
    Modal,
    Select,
    Skeleton,
    Space,
    Splitter,
    Table,
    Tag,
    Typography,
} from "antd";
import {InboxOutlined, SwapOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {useNavigate} from "react-router-dom";
import {
    AbaExplorerService,
    App as AppService,
    ConvertService,
} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import type {
    ConvertOutcome,
    UnpackedFile,
} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import PathHeader from "./common/PathHeader";
import EmptyState from "./common/EmptyState";
import VirtualTable, {VirtualColumn} from "./common/VirtualTable";
import FilePreviewPanel from "./unpacked/FilePreviewPanel";
import {setUnpackedDir, useWorkspace} from "../hooks/workspace";
import {useDebouncedValue} from "../hooks/useDebouncedValue";
import {openInModEditor} from "../hooks/modEditor";
import {TargetOrder, typeColor} from "../utils/consts";
import {appMessage as message, describeError} from "../utils/feedback";
import {baseName, formatBytes, formatNumber} from "../utils/format";

const {Text} = Typography;

/**
 * UnpackedPage 解包目录浏览与转换页
 * 解包产物按 Unity 类型分成一级目录（Texture2D、TextAsset、Mesh…），
 * 左侧列表可多选后批量转换，右侧展示选中文件的预览与可用转换目标
 */
const UnpackedPage: React.FC = () => {
    const {t} = useTranslation();
    const navigate = useNavigate();
    const {unpackedDir} = useWorkspace();
    const [files, setFiles] = useState<UnpackedFile[]>([]);
    const [loading, setLoading] = useState(false);
    const [search, setSearch] = useState("");
    // 过滤走防抖，解包目录动辄上千个文件，每次按键都重算会卡手
    const debouncedSearch = useDebouncedValue(search);
    const [kindFilter, setKindFilter] = useState<string[]>([]);
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [activePath, setActivePath] = useState<string>("");
    const [batchTarget, setBatchTarget] = useState<string>("json");
    const [converting, setConverting] = useState(false);
    const [outcomes, setOutcomes] = useState<ConvertOutcome[] | null>(null);

    const load = useCallback(async () => {
        if (!unpackedDir) {
            setFiles([]);
            return;
        }
        setLoading(true);
        try {
            const result = await AbaExplorerService.ListUnpackedDir(unpackedDir);
            setFiles(result?.files ?? []);
            setSelected(new Set());
            setActivePath("");
        } catch (error) {
            setFiles([]);
            message.error(describeError(error));
        } finally {
            setLoading(false);
        }
    }, [unpackedDir]);

    useEffect(() => {
        void load();
    }, [load]);

    const kindOptions = useMemo(() => {
        const counts = new Map<string, number>();
        files.forEach((file) => {
            const key = file.kind || t("UnpackedPage.root");
            counts.set(key, (counts.get(key) ?? 0) + 1);
        });
        return [...counts.entries()]
            .sort((left, right) => right[1] - left[1])
            .map(([kind, count]) => ({value: kind, label: `${kind} (${count})`}));
    }, [files, t]);

    const rows = useMemo(() => {
        const keyword = debouncedSearch.trim().toLowerCase();
        return files.filter((file) => {
            if (kindFilter.length > 0 && !kindFilter.includes(file.kind || t("UnpackedPage.root"))) return false;
            if (!keyword) return true;
            return file.relPath.toLowerCase().includes(keyword);
        });
    }, [files, debouncedSearch, kindFilter, t]);

    const activeFile = useMemo(
        () => files.find((file) => file.absPath === activePath) ?? null,
        [files, activePath]
    );

    const allVisibleSelected = rows.length > 0 && rows.every((file) => selected.has(file.absPath));

    const toggleAllVisible = () => {
        const next = new Set(selected);
        if (allVisibleSelected) {
            rows.forEach((file) => next.delete(file.absPath));
        } else {
            rows.forEach((file) => next.add(file.absPath));
        }
        setSelected(next);
    };

    const toggleOne = (absPath: string) => {
        const next = new Set(selected);
        if (next.has(absPath)) {
            next.delete(absPath);
        } else {
            next.add(absPath);
        }
        setSelected(next);
    };

    const convertSelected = async () => {
        if (selected.size === 0) return;
        setConverting(true);
        try {
            const outDir = await AppService.SelectDirectory(t("UnpackedPage.choose_output_dir"));
            if (!outDir) return;
            const result = await ConvertService.ConvertBatch([...selected], batchTarget, outDir);
            setOutcomes(result ?? []);
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setConverting(false);
        }
    };

    const openPackPage = () => {
        setUnpackedDir(unpackedDir);
        navigate("/pack");
    };

    if (!unpackedDir) {
        return (
            <EmptyState
                description={t("UnpackedPage.no_dir")}
                hint={t("UnpackedPage.no_dir_hint")}
                actionLabel={t("UnpackedPage.choose_dir")}
                onAction={async () => {
                    try {
                        const dir = await AppService.SelectDirectory(t("UnpackedPage.choose_dir"));
                        if (dir) setUnpackedDir(dir);
                    } catch (error) {
                        message.error(describeError(error));
                    }
                }}
            />
        );
    }

    const columns: VirtualColumn<UnpackedFile>[] = [
        {
            key: "select",
            title: <Checkbox checked={allVisibleSelected} onChange={toggleAllVisible}/>,
            width: "44px",
            align: "center",
            render: (row) => (
                <Checkbox
                    checked={selected.has(row.absPath)}
                    onChange={() => toggleOne(row.absPath)}
                    onClick={(event) => event.stopPropagation()}
                />
            ),
        },
        {
            key: "kind",
            title: t("UnpackedPage.kind"),
            width: "minmax(120px, 1fr)",
            sortValue: (row) => row.kind || "",
            render: (row) =>
                row.kind ? (
                    <Tag color={typeColor(row.kind)} style={{marginInlineEnd: 0}}>{row.kind}</Tag>
                ) : (
                    <Tag style={{marginInlineEnd: 0}}>{t("UnpackedPage.root")}</Tag>
                ),
        },
        {
            key: "name",
            title: t("UnpackedPage.file"),
            width: "minmax(200px, 3fr)",
            sortValue: (row) => baseName(row.relPath),
            render: (row) => baseName(row.relPath),
        },
        {
            key: "size",
            title: t("UnpackedPage.size"),
            width: "110px",
            align: "right",
            mono: true,
            sortValue: (row) => row.size,
            render: (row) => formatBytes(row.size),
        },
    ];

    return (
        <Card
            style={{flex: 1, minWidth: 0, display: "flex", flexDirection: "column"}}
            styles={{body: {flex: 1, minHeight: 0, display: "flex", flexDirection: "column", padding: 16}}}
        >
            <PathHeader
                path={unpackedDir}
                onReload={load}
                actions={
                    <Button icon={<InboxOutlined/>} onClick={openPackPage}>
                        {t("UnpackedPage.pack_this")}
                    </Button>
                }
            />

            <Space wrap style={{marginBottom: 12}}>
                <Input.Search
                    allowClear
                    placeholder={t("UnpackedPage.search_placeholder")}
                    style={{width: 260}}
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                />
                <Select
                    allowClear
                    mode="multiple"
                    maxTagCount="responsive"
                    placeholder={t("UnpackedPage.filter_kind")}
                    style={{minWidth: 200}}
                    options={kindOptions}
                    value={kindFilter}
                    onChange={setKindFilter}
                />
                <Select
                    style={{width: 140}}
                    value={batchTarget}
                    onChange={setBatchTarget}
                    options={TargetOrder.map((key) => ({value: key, label: t(`Target.${key}`)}))}
                />
                <Button
                    type="primary"
                    icon={<SwapOutlined/>}
                    disabled={selected.size === 0}
                    loading={converting}
                    onClick={() => void convertSelected()}
                >
                    {t("UnpackedPage.convert_selected", {count: selected.size})}
                </Button>
                <Text type="secondary">
                    {t("UnpackedPage.count", {shown: rows.length, total: files.length})}
                </Text>
                <Text type="secondary" style={{fontSize: 12}}>
                    {t("ModEditor.double_click_hint")}
                </Text>
            </Space>

            {loading && files.length === 0 ? (
                <Skeleton active paragraph={{rows: 8}}/>
            ) : (
                <Splitter style={{flex: 1, minHeight: 0}}>
                    <Splitter.Panel defaultSize="58%" min="30%">
                        <VirtualTable
                            rows={rows}
                            columns={columns}
                            rowKey={(row) => row.absPath}
                            defaultSort={{key: "name", order: "asc"}}
                            selectedKey={activePath}
                            onRowClick={(row) => setActivePath(row.absPath)}
                            onRowDoubleClick={(row) => void openInModEditor(row.absPath)}
                            emptyText={t("UnpackedPage.empty")}
                        />
                    </Splitter.Panel>
                    <Splitter.Panel min="25%">
                        <FilePreviewPanel file={activeFile}/>
                    </Splitter.Panel>
                </Splitter>
            )}

            <Modal
                open={outcomes !== null}
                title={t("UnpackedPage.batch_result")}
                width={760}
                footer={<Button type="primary" onClick={() => setOutcomes(null)}>{t("Common.close")}</Button>}
                onCancel={() => setOutcomes(null)}
            >
                <Text type="secondary" style={{display: "block", marginBottom: 8}}>
                    {t("UnpackedPage.batch_summary", {
                        ok: formatNumber((outcomes ?? []).filter((item) => !item.error).length),
                        failed: formatNumber((outcomes ?? []).filter((item) => item.error).length),
                    })}
                </Text>
                <Table
                    size="small"
                    pagination={{pageSize: 10, hideOnSinglePage: true}}
                    rowKey={(row) => row.inputPath}
                    dataSource={outcomes ?? []}
                    columns={[
                        {
                            title: t("UnpackedPage.result_file"),
                            dataIndex: "inputPath",
                            render: (value: string) => baseName(value),
                        },
                        {
                            title: t("UnpackedPage.result_status"),
                            key: "status",
                            width: 110,
                            render: (_, row) =>
                                row.error ? (
                                    <Tag color="red" style={{marginInlineEnd: 0}}>{t("UnpackedPage.failed")}</Tag>
                                ) : (
                                    <Tag color="green" style={{marginInlineEnd: 0}}>{t("UnpackedPage.ok")}</Tag>
                                ),
                        },
                        {
                            title: t("UnpackedPage.result_detail"),
                            key: "detail",
                            render: (_, row) =>
                                row.error ? (
                                    <Text type="danger" style={{fontSize: 12}}>{row.error}</Text>
                                ) : (
                                    <Text type="secondary" style={{fontSize: 12}}>{baseName(row.outputPath)}</Text>
                                ),
                        },
                    ]}
                />
            </Modal>
        </Card>
    );
};

export default UnpackedPage;
