import React, {useMemo, useState} from "react";
import {Input, Space, Typography} from "antd";
import {useTranslation} from "react-i18next";
import VirtualTable, {VirtualColumn} from "../common/VirtualTable";
import {useDebouncedValue} from "../../hooks/useDebouncedValue";
import {toBigInt} from "../../utils/bigint";
import type {CtCatalogItem} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import {formatNumber} from "../../utils/format";
import {CatalogItemTableWidthsKey} from "../../utils/LocalStorageKeys";

const {Text} = Typography;

interface CatalogItemTableProps {
    items: CtCatalogItem[];
    /** AssetBundle catalog 的资源文件名，用于把 resourceIndex 解析成文件名 */
    resourceFileNames: string[];
    /** virtualAsset catalog 展示 assetPath 而不是资源文件名 */
    virtual: boolean;
}

/**
 * CatalogItemTable catalog 条目表
 * 游戏用 hash（GetHashIgnoreCase 的结果）作为查找键，因此哈希列直接展示原值供排查加载失败
 * 整合包的条目数可达上万，走虚拟滚动
 */
const CatalogItemTable: React.FC<CatalogItemTableProps> = ({items, resourceFileNames, virtual}) => {
    const {t} = useTranslation();
    const [search, setSearch] = useState("");
    // 过滤走防抖，避免每次按键都重算整张表
    const debouncedSearch = useDebouncedValue(search);

    const rows = useMemo(() => {
        const keyword = debouncedSearch.trim().toLowerCase();
        if (!keyword) return items;
        return items.filter(
            (item) =>
                item.name.toLowerCase().includes(keyword) ||
                item.hash.includes(keyword) ||
                item.assetPath.toLowerCase().includes(keyword)
        );
    }, [items, debouncedSearch]);

    const columns: VirtualColumn<CtCatalogItem>[] = [
        {
            key: "name",
            title: t("CatalogItemTable.name"),
            width: "minmax(220px, 3fr)",
            sortValue: (row) => row.name,
            render: (row) => row.name || <Text type="secondary">{t("CatalogItemTable.unnamed")}</Text>,
        },
        {
            key: "hash",
            title: t("CatalogItemTable.hash"),
            width: "minmax(180px, 2fr)",
            mono: true,
            sortValue: (row) => toBigInt(row.hash),
            render: (row) => row.hash,
        },
        virtual
            ? {
                key: "assetPath",
                title: t("CatalogItemTable.asset_path"),
                width: "minmax(220px, 3fr)",
                sortValue: (row) => row.assetPath,
                render: (row) => row.assetPath,
            }
            : {
                key: "resource",
                title: t("CatalogItemTable.resource"),
                width: "minmax(180px, 2fr)",
                sortValue: (row) => resourceFileNames[row.resourceIndex] ?? `#${row.resourceIndex}`,
                render: (row) =>
                    resourceFileNames[row.resourceIndex] ?? `#${formatNumber(row.resourceIndex)}`,
            },
    ];

    return (
        <div style={{display: "flex", flexDirection: "column", height: "100%", minHeight: 0, gap: 12}}>
            <Space wrap>
                <Input.Search
                    allowClear
                    placeholder={t("CatalogItemTable.search_placeholder")}
                    style={{width: 320}}
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                />
                <Text type="secondary">{t("CatalogItemTable.count", {shown: rows.length, total: items.length})}</Text>
            </Space>
            <div style={{flex: 1, minHeight: 0}}>
                <VirtualTable
                    rows={rows}
                    columns={columns}
                    rowKey={(row, index) => `${row.hash}:${index}`}
                    defaultSort={{key: "name", order: "asc"}}
                    widthStorageKey={CatalogItemTableWidthsKey}
                    emptyText={t("CatalogItemTable.empty")}
                />
            </div>
        </div>
    );
};

export default CatalogItemTable;
