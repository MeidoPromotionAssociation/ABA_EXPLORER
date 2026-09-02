import React, {useCallback, useEffect, useMemo, useRef, useState} from "react";
import {
    Alert,
    Button,
    Card,
    Collapse,
    Input,
    Progress,
    Select,
    Space,
    Switch,
    Tag,
    Tooltip,
    Typography,
} from "antd";
import {
    DatabaseOutlined,
    FolderOpenOutlined,
    ReloadOutlined,
    SearchOutlined,
    StopOutlined,
    TableOutlined,
    UnorderedListOutlined,
} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {useNavigate} from "react-router-dom";
import {Events} from "@wailsio/runtime";
import {
    App as AppService,
    SearchService,
} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import type {
    IndexFacets,
    IndexProgress,
    IndexStats,
    SearchHit,
} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import VirtualTable, {VirtualColumn} from "./common/VirtualTable";
import {setContainerPath, setCtPath} from "../hooks/workspace";
import {useDebouncedValue} from "../hooks/useDebouncedValue";
import {appMessage as message, describeError} from "../utils/feedback";
import {formatNumber} from "../utils/format";
import {SearchDeepKey, SearchHitTableWidthsKey, SearchRootKey} from "../utils/LocalStorageKeys";

const {Text, Paragraph} = Typography;

// 来源层的 Tag 配色，与容器页的类型配色区分开，一眼能看出名字是从哪一层挖出来的
// Tag colors per origin layer, kept distinct from the container page's type colors so the layer a name came from reads at a glance
const OriginColors: Record<string, string> = {
    catalog: "blue",
    container: "geekblue",
    menu: "magenta",
    material: "gold",
    pmat: "orange",
};

// 一次查询最多取回的条数，与后端 maxSearchLimit 保持一致
// The most hits one query fetches, kept in step with maxSearchLimit on the backend
const HitLimit = 5000;

/**
 * SearchPage 全局资源名搜索页
 *
 * 游戏按名字加载资源，而名字分散在两层：.ct 的 catalog 列出容器里的资源，
 * .menu/.mate/.pmat 这类名字只存在于容器内部的 .menuassets/.materialassets/.pmatassets 里，
 * catalog 根本不会列。所以"深度索引"开着才能回答"这个 .menu 在哪个 aba 里"。
 */
const SearchPage: React.FC = () => {
    const {t} = useTranslation();
    const navigate = useNavigate();

    const [root, setRoot] = useState(() => localStorage.getItem(SearchRootKey) ?? "");
    const [deep, setDeep] = useState(() => localStorage.getItem(SearchDeepKey) !== "false");
    const [stats, setStats] = useState<IndexStats | null>(null);
    const [progress, setProgress] = useState<IndexProgress | null>(null);
    const [building, setBuilding] = useState(false);
    const [facets, setFacets] = useState<IndexFacets | null>(null);
    const [restoring, setRestoring] = useState(false);

    const [keyword, setKeyword] = useState("");
    const debouncedKeyword = useDebouncedValue(keyword, 250);
    const [extensions, setExtensions] = useState<string[]>([]);
    const [origins, setOrigins] = useState<string[]>([]);
    const [hits, setHits] = useState<SearchHit[]>([]);
    const [total, setTotal] = useState(0);
    const [truncated, setTruncated] = useState(false);
    const [searching, setSearching] = useState(false);

    // 查询是异步的，慢的那次回来晚了会覆盖新结果，用序号丢弃过期响应
    // Queries are async and a slow one returning late would overwrite newer results, so a sequence number discards stale responses
    const querySeq = useRef(0);

    const refreshStatus = useCallback(async () => {
        try {
            const [status, currentFacets] = await Promise.all([
                SearchService.IndexStatus(),
                SearchService.Facets(),
            ]);
            setStats(status);
            setFacets(currentFacets);
            setBuilding(status?.building ?? false);
            // 只在本地没有选择时补上后端记录的根目录：用户刚选好还没建索引就切走再回来，
            // 这里若无条件覆盖会把他的选择抹掉换成上一次索引的目录
            // Only fill in the backend's root when there is no local choice: if the user picks a directory,
            // navigates away before building and comes back, an unconditional overwrite would replace
            // their pick with the previously indexed directory
            if (status?.root) setRoot((current) => current || status.root);
            return status;
        } catch (error) {
            console.warn("read index status failed:", error);
            return null;
        }
    }, []);

    // 索引常驻后端，切走再回来要把状态读回来，否则页面看起来像没建过索引
    // 后端还没有索引时再试一次磁盘缓存：全部文件都没变过就能直接恢复，用户不必重新等一次扫描
    // The index lives in the backend, so navigating away and back must restore the status or the page looks unindexed
    // With no index in the backend the on-disk cache is tried as well, which restores instantly when no file
    // changed and spares the user another scan
    useEffect(() => {
        let cancelled = false;
        (async () => {
            const status = await refreshStatus();
            if (cancelled || status?.ready || status?.building) return;
            const savedRoot = localStorage.getItem(SearchRootKey) ?? "";
            if (!savedRoot) return;
            try {
                setRestoring(true);
                const loaded = await SearchService.LoadCachedIndex({root: savedRoot, deep, refresh: false});
                if (cancelled || !loaded?.ready) return;
                setStats(loaded);
                setFacets(await SearchService.Facets());
            } catch (error) {
                console.warn("restore the cached index failed:", error);
            } finally {
                if (!cancelled) setRestoring(false);
            }
        })();
        return () => {
            cancelled = true;
        };
        // 只在挂载时恢复一次，deep 开关变化不该悄悄换掉已经装好的索引
        // Restore only once on mount, since flipping deep must not silently swap an already loaded index
    }, []);

    useEffect(() => {
        const off = Events.On("explorer:index-progress", (event: any) => {
            const data = event?.data;
            const update: IndexProgress | undefined = Array.isArray(data) ? data[0] : data;
            if (!update) return;
            setProgress(update.finished ? null : update);
        });
        return () => {
            off();
        };
    }, []);

    const runSearch = useCallback(async () => {
        const seq = ++querySeq.current;
        setSearching(true);
        try {
            const result = await SearchService.Search({
                text: debouncedKeyword,
                extensions,
                origins,
                limit: HitLimit,
            });
            if (seq !== querySeq.current) return;
            setHits(result?.hits ?? []);
            setTotal(result?.total ?? 0);
            setTruncated(result?.truncated ?? false);
        } catch (error) {
            if (seq === querySeq.current) message.error(describeError(error));
        } finally {
            if (seq === querySeq.current) setSearching(false);
        }
    }, [debouncedKeyword, extensions, origins]);

    useEffect(() => {
        if (!stats?.ready) {
            setHits([]);
            setTotal(0);
            return;
        }
        void runSearch();
    }, [runSearch, stats?.ready, stats?.names]);

    const chooseRoot = async () => {
        try {
            const directory = await AppService.SelectDirectory(t("SearchPage.choose_root"));
            if (!directory) return;
            setRoot(directory);
            localStorage.setItem(SearchRootKey, directory);
        } catch (error) {
            message.error(describeError(error));
        }
    };

    const buildIndex = async (refresh = false) => {
        if (!root) {
            message.warning(t("SearchPage.no_root"));
            return;
        }
        localStorage.setItem(SearchRootKey, root);
        localStorage.setItem(SearchDeepKey, String(deep));
        setBuilding(true);
        setProgress(null);
        try {
            const result = await SearchService.BuildIndex({root, deep, refresh});
            setStats(result);
            setFacets(await SearchService.Facets());
            if (result?.cancelled) {
                message.info(t("SearchPage.build_cancelled"));
            } else {
                message.success(t("SearchPage.build_done", {
                    names: formatNumber(result?.names ?? 0),
                    seconds: ((result?.elapsedMs ?? 0) / 1000).toFixed(1),
                }));
            }
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setBuilding(false);
            setProgress(null);
        }
    };

    const cancelBuild = async () => {
        try {
            await SearchService.CancelIndex();
        } catch (error) {
            message.error(describeError(error));
        }
    };

    const openContainer = (hit: SearchHit) => {
        if (!hit.containerPath) {
            message.warning(t("SearchPage.no_container_path"));
            return;
        }
        setContainerPath(hit.containerPath);
        navigate("/container");
    };

    const openCatalog = (hit: SearchHit) => {
        if (!hit.catalogPath) return;
        setCtPath(hit.catalogPath);
        navigate("/ct");
    };

    const reveal = async (hit: SearchHit) => {
        const target = hit.containerPath || hit.catalogPath;
        if (!target) return;
        try {
            await AppService.Reveal(target);
        } catch (error) {
            message.error(describeError(error));
        }
    };

    const extensionOptions = useMemo(
        () => (facets?.extensions ?? []).map((facet) => ({
            value: facet.value,
            label: `${facet.value || t("SearchPage.no_extension")} (${formatNumber(facet.count)})`,
        })),
        [facets, t]
    );

    const originOptions = useMemo(
        () => (facets?.origins ?? []).map((facet) => ({
            value: facet.value,
            label: `${t(`Origin.${facet.value}`)} (${formatNumber(facet.count)})`,
        })),
        [facets, t]
    );

    const columns: VirtualColumn<SearchHit>[] = [
        {
            key: "name",
            title: t("SearchPage.name"),
            width: "minmax(220px, 3fr)",
            sortValue: (row) => row.name,
            render: (row) => <Text ellipsis={{tooltip: row.name}}>{row.name}</Text>,
        },
        {
            key: "origin",
            title: t("SearchPage.origin"),
            width: "minmax(110px, 1fr)",
            sortValue: (row) => row.origin,
            render: (row) => (
                <Tag color={OriginColors[row.origin]} style={{marginInlineEnd: 0}}>
                    {t(`Origin.${row.origin}`)}
                </Tag>
            ),
        },
        {
            key: "detail",
            title: t("SearchPage.detail"),
            width: "minmax(140px, 2fr)",
            sortValue: (row) => row.detail,
            render: (row) => (
                <Text type="secondary" ellipsis={{tooltip: row.detail}} style={{fontSize: 12}}>
                    {row.detail}
                </Text>
            ),
        },
        {
            key: "container",
            title: t("SearchPage.container"),
            width: "minmax(180px, 2fr)",
            sortValue: (row) => row.containerName,
            render: (row) => (
                <Text ellipsis={{tooltip: row.containerPath || row.containerName}}>
                    {row.containerName || "-"}
                </Text>
            ),
        },
        {
            key: "owner",
            title: t("SearchPage.owner"),
            width: "minmax(150px, 2fr)",
            sortValue: (row) => row.owner,
            render: (row) => (
                <Text type="secondary" ellipsis={{tooltip: row.owner}} style={{fontSize: 12}}>
                    {row.owner}
                </Text>
            ),
        },
        {
            key: "actions",
            title: t("SearchPage.actions"),
            width: "116px",
            align: "center",
            resizable: false,
            render: (row) => (
                <Space size={4}>
                    <Tooltip title={t("SearchPage.open_container")}>
                        <Button
                            type="text"
                            size="small"
                            icon={<TableOutlined/>}
                            disabled={!row.containerPath}
                            onClick={(event) => {
                                event.stopPropagation();
                                openContainer(row);
                            }}
                        />
                    </Tooltip>
                    <Tooltip title={t("SearchPage.open_catalog")}>
                        <Button
                            type="text"
                            size="small"
                            icon={<UnorderedListOutlined/>}
                            disabled={!row.catalogPath}
                            onClick={(event) => {
                                event.stopPropagation();
                                openCatalog(row);
                            }}
                        />
                    </Tooltip>
                    <Tooltip title={t("Common.reveal")}>
                        <Button
                            type="text"
                            size="small"
                            icon={<FolderOpenOutlined/>}
                            disabled={!row.containerPath && !row.catalogPath}
                            onClick={(event) => {
                                event.stopPropagation();
                                void reveal(row);
                            }}
                        />
                    </Tooltip>
                </Space>
            ),
        },
    ];

    const percent = progress?.total ? Math.round((progress.done / progress.total) * 100) : 0;

    return (
        <Card
            style={{flex: 1, minWidth: 0, display: "flex", flexDirection: "column"}}
            styles={{body: {flex: 1, minHeight: 0, display: "flex", flexDirection: "column", padding: 16}}}
        >
            <Space wrap style={{marginBottom: 12}}>
                <Button icon={<FolderOpenOutlined/>} onClick={chooseRoot}>
                    {t("SearchPage.choose_root")}
                </Button>
                <Text type="secondary" style={{maxWidth: 420}} ellipsis={{tooltip: root}}>
                    {root || t("SearchPage.no_root_yet")}
                </Text>
                <Tooltip title={t("SearchPage.deep_tip")}>
                    <Space size={6}>
                        <Switch
                            size="small"
                            checked={deep}
                            disabled={building}
                            onChange={(checked) => {
                                setDeep(checked);
                                localStorage.setItem(SearchDeepKey, String(checked));
                            }}
                        />
                        <Text style={{fontSize: 13}}>{t("SearchPage.deep")}</Text>
                    </Space>
                </Tooltip>
                <Tooltip title={t("SearchPage.build_tip")}>
                    <Button
                        type="primary"
                        icon={<DatabaseOutlined/>}
                        loading={building || restoring}
                        disabled={!root}
                        onClick={() => void buildIndex(false)}
                    >
                        {stats?.ready ? t("SearchPage.update") : t("SearchPage.build")}
                    </Button>
                </Tooltip>
                {stats?.ready && !building && (
                    <Tooltip title={t("SearchPage.rebuild_tip")}>
                        <Button icon={<ReloadOutlined/>} onClick={() => void buildIndex(true)}>
                            {t("SearchPage.rebuild")}
                        </Button>
                    </Tooltip>
                )}
                {building && (
                    <Button danger icon={<StopOutlined/>} onClick={() => void cancelBuild()}>
                        {t("Common.cancel")}
                    </Button>
                )}
            </Space>

            {building && (
                <div style={{marginBottom: 12}}>
                    <Progress
                        percent={percent}
                        status="active"
                        format={() =>
                            progress
                                ? `${formatNumber(progress.done)} / ${formatNumber(progress.total)}`
                                : t("SearchPage.scanning")
                        }
                    />
                    <Text type="secondary" style={{fontSize: 12}} ellipsis>
                        {progress?.current
                            ? t("SearchPage.progress_detail", {
                                file: progress.current,
                                names: formatNumber(progress.names),
                            })
                            : t("SearchPage.scanning")}
                    </Text>
                </div>
            )}

            {stats?.ready && !building && (
                <Text type="secondary" style={{fontSize: 12, marginBottom: 12}}>
                    {t("SearchPage.index_summary", {
                        names: formatNumber(stats.names),
                        inner: formatNumber(stats.innerNames),
                        containers: formatNumber(stats.containers),
                        catalogs: formatNumber(stats.catalogs),
                        seconds: (stats.elapsedMs / 1000).toFixed(1),
                    })}
                    {stats.fromCache
                        ? ` ${t("SearchPage.from_cache")}`
                        : stats.reused > 0
                            ? ` ${t("SearchPage.reused", {count: stats.reused})}`
                            : ""}
                </Text>
            )}

            {stats?.ready && !stats.deep && (
                <Alert
                    type="info"
                    showIcon
                    style={{marginBottom: 12}}
                    title={t("SearchPage.shallow_warning")}
                />
            )}

            {(stats?.warnings?.length ?? 0) > 0 && (
                <Collapse
                    ghost
                    size="small"
                    style={{marginBottom: 12}}
                    items={[{
                        key: "warnings",
                        label: (
                            <Text type="warning" style={{fontSize: 12}}>
                                {t("SearchPage.warnings", {count: stats?.warnings?.length ?? 0})}
                            </Text>
                        ),
                        children: (
                            <div style={{maxHeight: 160, overflow: "auto"}}>
                                {(stats?.warnings ?? []).map((warning) => (
                                    <Paragraph key={warning} type="secondary" style={{fontSize: 12, marginBottom: 4}}>
                                        {warning}
                                    </Paragraph>
                                ))}
                            </div>
                        ),
                    }]}
                />
            )}

            <Space wrap style={{marginBottom: 12}}>
                <Input
                    allowClear
                    prefix={<SearchOutlined/>}
                    placeholder={t("SearchPage.search_placeholder")}
                    style={{width: 320}}
                    value={keyword}
                    disabled={!stats?.ready}
                    onChange={(event) => setKeyword(event.target.value)}
                />
                <Select
                    allowClear
                    mode="multiple"
                    maxTagCount="responsive"
                    placeholder={t("SearchPage.filter_extension")}
                    style={{minWidth: 190}}
                    disabled={!stats?.ready}
                    options={extensionOptions}
                    value={extensions}
                    onChange={setExtensions}
                />
                <Select
                    allowClear
                    mode="multiple"
                    maxTagCount="responsive"
                    placeholder={t("SearchPage.filter_origin")}
                    style={{minWidth: 190}}
                    disabled={!stats?.ready}
                    options={originOptions}
                    value={origins}
                    onChange={setOrigins}
                />
                {stats?.ready && (
                    <Text type="secondary">
                        {truncated
                            ? t("SearchPage.count_truncated", {
                                shown: formatNumber(hits.length),
                                total: formatNumber(total),
                            })
                            : t("SearchPage.count", {total: formatNumber(total)})}
                    </Text>
                )}
            </Space>

            {!stats?.ready ? (
                <div style={{flex: 1, display: "flex", alignItems: "center", justifyContent: "center"}}>
                    <div style={{maxWidth: 520, textAlign: "center"}}>
                        <Paragraph>{t("SearchPage.intro")}</Paragraph>
                        <Paragraph type="secondary" style={{fontSize: 12}}>
                            {t("SearchPage.intro_hint")}
                        </Paragraph>
                    </div>
                </div>
            ) : (
                <VirtualTable
                    rows={hits}
                    columns={columns}
                    rowKey={(row, index) => `${row.containerPath} ${row.owner} ${row.name} ${index}`}
                    widthStorageKey={SearchHitTableWidthsKey}
                    onRowDoubleClick={(row) => openContainer(row)}
                    emptyText={searching ? t("SearchPage.searching") : t("SearchPage.empty")}
                />
            )}
        </Card>
    );
};

export default SearchPage;
