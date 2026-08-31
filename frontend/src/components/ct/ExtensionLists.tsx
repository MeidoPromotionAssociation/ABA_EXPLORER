import React, {useEffect, useMemo, useState} from "react";
import {Empty, Input, Select, Space, Typography} from "antd";
import {useTranslation} from "react-i18next";
import VirtualTable, {VirtualColumn} from "../common/VirtualTable";
import {useDebouncedValue} from "../../hooks/useDebouncedValue";
import {toBigInt} from "../../utils/bigint";
import {ExtensionListWidthsKey} from "../../utils/LocalStorageKeys";
import type {
    CtExtensionName,
    CtExtensionNameList,
} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";

const {Text} = Typography;

/**
 * ExtensionLists 按扩展名分组的名称表
 * 官方 system.ct 把无后缀资源的分组命名为 null，因此分组键可能不是真正的扩展名
 */
const ExtensionLists: React.FC<{ lists: CtExtensionNameList[] }> = ({lists}) => {
    const {t} = useTranslation();
    const [selected, setSelected] = useState<string>(lists[0]?.key ?? "");
    const [search, setSearch] = useState("");
    // 过滤走防抖，避免每次按键都重算整张表
    const debouncedSearch = useDebouncedValue(search);

    // 换文件后原来选中的分组可能已不存在，回落到第一个分组
    useEffect(() => {
        if (!lists.some((list) => list.key === selected)) {
            setSelected(lists[0]?.key ?? "");
        }
    }, [lists, selected]);

    const active = lists.find((list) => list.key === selected);

    const rows = useMemo(() => {
        const data = active?.data ?? [];
        const keyword = debouncedSearch.trim().toLowerCase();
        if (!keyword) return data;
        return data.filter((item) => item.name.toLowerCase().includes(keyword) || item.hash.includes(keyword));
    }, [active, debouncedSearch]);

    const columns: VirtualColumn<CtExtensionName>[] = [
        {
            key: "name",
            title: t("ExtensionLists.name"),
            width: "minmax(240px, 3fr)",
            sortValue: (row) => row.name,
            render: (row) => row.name || <Text type="secondary">{t("ExtensionLists.unnamed")}</Text>,
        },
        {
            key: "hash",
            title: t("ExtensionLists.hash"),
            width: "minmax(180px, 2fr)",
            mono: true,
            sortValue: (row) => toBigInt(row.hash),
            render: (row) => row.hash,
        },
    ];

    if (lists.length === 0) {
        return <Empty description={t("ExtensionLists.empty")} image={Empty.PRESENTED_IMAGE_SIMPLE}/>;
    }

    return (
        <div style={{display: "flex", flexDirection: "column", height: "100%", minHeight: 0, gap: 12}}>
            <Space wrap>
                <Select
                    style={{minWidth: 220}}
                    value={selected || undefined}
                    onChange={setSelected}
                    options={lists.map((list) => ({
                        value: list.key,
                        label: `${list.key} (${(list.data ?? []).length})`,
                    }))}
                />
                <Input.Search
                    allowClear
                    placeholder={t("ExtensionLists.search_placeholder")}
                    style={{width: 280}}
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                />
                {active && active.extension !== active.key && (
                    <Text type="secondary">{t("ExtensionLists.extension_field", {value: active.extension || "-"})}</Text>
                )}
            </Space>
            <div style={{flex: 1, minHeight: 0}}>
                <VirtualTable
                    rows={rows}
                    columns={columns}
                    rowKey={(row, index) => `${row.hash}:${index}`}
                    defaultSort={{key: "name", order: "asc"}}
                    widthStorageKey={ExtensionListWidthsKey}
                    emptyText={t("ExtensionLists.no_entries")}
                />
            </div>
        </div>
    );
};

export default ExtensionLists;
