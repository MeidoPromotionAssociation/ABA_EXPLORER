import React, {useMemo, useState} from "react";
import {Button, Input, Select, Space, Tag, Typography} from "antd";
import {DownloadOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import VirtualTable, {VirtualColumn} from "../common/VirtualTable";
import {useDebouncedValue} from "../../hooks/useDebouncedValue";
import {
    AbaExplorerService,
    App as AppService,
} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import type {AbaSerializedFile} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import {typeColor} from "../../utils/consts";
import {AssetTableWidthsKey} from "../../utils/LocalStorageKeys";
import {toBigInt} from "../../utils/bigint";
import {appMessage as message, describeError} from "../../utils/feedback";
import {baseName, formatBytes, formatNumber} from "../../utils/format";

const {Text} = Typography;

/** FlatAsset 一条被拉平的对象记录，附带它来自哪个 SerializedFile */
interface FlatAsset {
    sourceIndex: number;
    sourceName: string;
    pathId: string;
    /** PathID 的 bigint 形式，排序时用；Unity PathID 是 64 位值，字符串比较会得到字典序 */
    pathIdValue: bigint;
    typeId: number;
    typeName: string;
    name: string;
    size: number;
    offset: number;
    container: string;
}

interface AssetTableProps {
    containerPath: string;
    serializedFiles: AbaSerializedFile[];
}

// 能单独导出的对象类型，与后端 AssetExportKind 的判断对应
// 其余类型（Mesh、AnimationClip 等）要重建独立 Unity 对象，交给整体解包
// Object types that can be exported on their own, matching AssetExportKind on the backend
// Other types such as Mesh and AnimationClip need a rebuilt standalone Unity object and go through a full unpack
const ExportableTypes = new Set(["TextAsset", "Texture2D", "Sprite"]);

/** suggestExportName 给出导出时预填的文件名，图像类换成 .png，KCES 的贴图名本身常带 .tex */
function suggestExportName(asset: FlatAsset): string {
    const name = asset.name || `asset_${asset.pathId}`;
    if (asset.typeName === "TextAsset") return name;
    return name.replace(/\.tex$/i, "") + ".png";
}

/**
 * AssetTable 容器内全部 Unity 对象的表格
 * 一个 ABA 可以有多个 SerializedFile，这里拉平成一张表并保留来源列，
 * 行数可达上万，所以交给 VirtualTable 只渲染视口内的行
 */
const AssetTable: React.FC<AssetTableProps> = ({containerPath, serializedFiles}) => {
    const {t} = useTranslation();
    const [search, setSearch] = useState("");
    // 上万行的表格不能每次按键都重新过滤，输入框保持即时响应、过滤走防抖后的值
    const debouncedSearch = useDebouncedValue(search);
    const [typeFilter, setTypeFilter] = useState<string[]>([]);
    const [sourceFilter, setSourceFilter] = useState<number[]>([]);
    const [exporting, setExporting] = useState("");

    // 导出单个对象：TextAsset 得到可直接使用的 KCES 文件，贴图与精灵解码成 PNG
    // Exporting one object: a TextAsset yields a usable KCES file while textures and sprites decode to PNG
    const exportAsset = async (asset: FlatAsset) => {
        if (!ExportableTypes.has(asset.typeName)) {
            message.info(t("AssetTable.export_unsupported"));
            return;
        }
        setExporting(asset.pathId);
        try {
            const suggested = suggestExportName(asset);
            const png = asset.typeName !== "TextAsset";
            const target = await AppService.SelectPathToSave(
                png ? "*.png" : "*.*",
                png ? t("AssetTable.png_filter") : t("AssetTable.any_filter"),
                suggested
            );
            if (!target) return;
            await AbaExplorerService.ExportAsset(containerPath, asset.sourceName, asset.pathId, target);
            message.success(t("AssetTable.exported", {name: baseName(target)}));
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setExporting("");
        }
    };

    const allAssets = useMemo<FlatAsset[]>(() => {
        const result: FlatAsset[] = [];
        serializedFiles.forEach((file, sourceIndex) => {
            (file.assets ?? []).forEach((asset) => {
                result.push({
                    sourceIndex,
                    sourceName: file.name,
                    pathId: asset.pathId,
                    pathIdValue: toBigInt(asset.pathId),
                    typeId: asset.typeId,
                    typeName: asset.typeName,
                    name: asset.name,
                    size: asset.size,
                    offset: asset.offset,
                    container: asset.container,
                });
            });
        });
        return result;
    }, [serializedFiles]);

    const typeOptions = useMemo(() => {
        const counts = new Map<string, number>();
        allAssets.forEach((asset) => {
            const key = asset.typeName || `Type_${asset.typeId}`;
            counts.set(key, (counts.get(key) ?? 0) + 1);
        });
        return [...counts.entries()]
            .sort((left, right) => right[1] - left[1])
            .map(([name, count]) => ({value: name, label: `${name} (${count})`}));
    }, [allAssets]);

    const rows = useMemo(() => {
        const keyword = debouncedSearch.trim().toLowerCase();
        return allAssets.filter((asset) => {
            if (typeFilter.length > 0 && !typeFilter.includes(asset.typeName || `Type_${asset.typeId}`)) return false;
            if (sourceFilter.length > 0 && !sourceFilter.includes(asset.sourceIndex)) return false;
            if (!keyword) return true;
            return (
                asset.name.toLowerCase().includes(keyword) ||
                asset.container.toLowerCase().includes(keyword) ||
                asset.pathId.includes(keyword) ||
                asset.typeName.toLowerCase().includes(keyword)
            );
        });
    }, [allAssets, debouncedSearch, typeFilter, sourceFilter]);

    const columns: VirtualColumn<FlatAsset>[] = [
        {
            key: "name",
            title: t("AssetTable.name"),
            width: "minmax(180px, 2fr)",
            sortValue: (row) => row.name,
            render: (row) => row.name || <Text type="secondary">{t("AssetTable.unnamed")}</Text>,
        },
        {
            key: "type",
            title: t("AssetTable.type"),
            width: "minmax(140px, 1fr)",
            sortValue: (row) => row.typeName || `Type_${row.typeId}`,
            render: (row) => (
                <Tag color={typeColor(row.typeName)} style={{marginInlineEnd: 0}}>
                    {row.typeName || `Type_${row.typeId}`}
                </Tag>
            ),
        },
        {
            key: "pathId",
            title: t("AssetTable.path_id"),
            width: "minmax(150px, 1fr)",
            mono: true,
            sortValue: (row) => row.pathIdValue,
            render: (row) => row.pathId,
        },
        {
            key: "size",
            title: t("AssetTable.size"),
            width: "110px",
            align: "right",
            mono: true,
            sortValue: (row) => row.size,
            render: (row) => formatBytes(row.size),
        },
        {
            key: "offset",
            title: t("AssetTable.offset"),
            width: "110px",
            align: "right",
            mono: true,
            sortValue: (row) => row.offset,
            render: (row) => formatNumber(row.offset),
        },
        {
            key: "container",
            title: t("AssetTable.container"),
            width: "minmax(160px, 2fr)",
            sortValue: (row) => row.container,
            render: (row) => row.container || "",
        },
    ];

    // 只有多个 SerializedFile 时才需要来源列与来源过滤
    if (serializedFiles.length > 1) {
        columns.push({
            key: "source",
            title: t("AssetTable.source"),
            width: "minmax(120px, 1fr)",
            sortValue: (row) => row.sourceName,
            render: (row) => row.sourceName,
        });
    }

    columns.push({
        key: "actions",
        title: "",
        width: "100px",
        align: "center",
        resizable: false,
        render: (row) =>
            ExportableTypes.has(row.typeName) ? (
                <Button
                    size="small"
                    icon={<DownloadOutlined/>}
                    loading={exporting === row.pathId}
                    onClick={(event) => {
                        event.stopPropagation();
                        void exportAsset(row);
                    }}
                >
                    {t("AssetTable.export")}
                </Button>
            ) : (
                ""
            ),
    });

    return (
        <div style={{display: "flex", flexDirection: "column", height: "100%", minHeight: 0, gap: 12}}>
            <Space wrap>
                <Input.Search
                    allowClear
                    placeholder={t("AssetTable.search_placeholder")}
                    style={{width: 300}}
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                />
                <Select
                    allowClear
                    mode="multiple"
                    maxTagCount="responsive"
                    placeholder={t("AssetTable.filter_type")}
                    style={{minWidth: 220}}
                    options={typeOptions}
                    value={typeFilter}
                    onChange={setTypeFilter}
                />
                {serializedFiles.length > 1 && (
                    <Select
                        allowClear
                        mode="multiple"
                        maxTagCount="responsive"
                        placeholder={t("AssetTable.filter_source")}
                        style={{minWidth: 200}}
                        options={serializedFiles.map((file, index) => ({value: index, label: file.name}))}
                        value={sourceFilter}
                        onChange={setSourceFilter}
                    />
                )}
                <Text type="secondary">
                    {t("AssetTable.count", {shown: rows.length, total: allAssets.length})}
                </Text>
                <Text type="secondary" style={{fontSize: 12}}>{t("AssetTable.export_hint")}</Text>
            </Space>
            <div style={{flex: 1, minHeight: 0}}>
                <VirtualTable
                    rows={rows}
                    columns={columns}
                    rowKey={(row) => `${row.sourceIndex}:${row.pathId}`}
                    defaultSort={{key: "name", order: "asc"}}
                    widthStorageKey={AssetTableWidthsKey}
                    onRowDoubleClick={(row) => void exportAsset(row)}
                    emptyText={t("AssetTable.empty")}
                />
            </div>
        </div>
    );
};

export default AssetTable;
