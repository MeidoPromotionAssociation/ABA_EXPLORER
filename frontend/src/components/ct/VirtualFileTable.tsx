import React, {useMemo, useState} from "react";
import {Button, Input, Space, Table, Tag, Typography} from "antd";
import {DownloadOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {
    App as AppService,
    CtExplorerService,
} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import type {CtVirtualFile} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import {appMessage as message, describeError} from "../../utils/feedback";
import {useDebouncedValue} from "../../hooks/useDebouncedValue";
import {baseName, formatBytes, formatNumber} from "../../utils/format";

const {Text} = Typography;

// numeric 让 ExtensionNameList2 排在 ExtensionNameList10 之前
const nameCollator = new Intl.Collator(undefined, {numeric: true, sensitivity: "base"});

interface VirtualFileTableProps {
    ctPath: string;
    files: CtVirtualFile[];
    /** catalog 与 ExtensionNameList 已在其他标签页解码展示，这里标记出来 */
    decodedNames: string[];
}

/**
 * VirtualFileTable .ct 内虚拟文件表
 * 虚拟文件通常只有 catalog 与几个 ExtensionNameList，数量小，用 antd Table 即可
 */
const VirtualFileTable: React.FC<VirtualFileTableProps> = ({ctPath, files, decodedNames}) => {
    const {t} = useTranslation();
    const [search, setSearch] = useState("");
    // 过滤走防抖，避免每次按键都重算整张表
    const debouncedSearch = useDebouncedValue(search);
    const [busyName, setBusyName] = useState("");

    const rows = useMemo(() => {
        const keyword = debouncedSearch.trim().toLowerCase();
        if (!keyword) return files;
        return files.filter((file) => file.name.toLowerCase().includes(keyword));
    }, [files, debouncedSearch]);

    const extract = async (file: CtVirtualFile) => {
        setBusyName(file.name);
        try {
            const target = await AppService.SelectPathToSave("*.*", t("VirtualFileTable.save_filter"), file.name);
            if (!target) return;
            await CtExplorerService.ExtractVirtualFile(ctPath, file.name, target);
            message.success(t("VirtualFileTable.extracted", {name: baseName(target)}));
        } catch (error) {
            message.error(describeError(error));
        } finally {
            setBusyName("");
        }
    };

    return (
        <div style={{display: "flex", flexDirection: "column", height: "100%", minHeight: 0, gap: 12}}>
            <Space wrap>
                <Input.Search
                    allowClear
                    placeholder={t("VirtualFileTable.search_placeholder")}
                    style={{width: 280}}
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                />
                <Text type="secondary">{t("VirtualFileTable.count", {count: files.length})}</Text>
            </Space>
            <div style={{flex: 1, minHeight: 0, overflow: "auto"}}>
                <Table
                    size="small"
                    pagination={false}
                    rowKey={(row) => row.name}
                    dataSource={rows}
                    locale={{emptyText: t("VirtualFileTable.empty")}}
                    columns={[
                        {
                            title: t("VirtualFileTable.name"),
                            dataIndex: "name",
                            defaultSortOrder: "ascend",
                            sorter: (left, right) => nameCollator.compare(left.name, right.name),
                            render: (value: string) => (
                                <Space size={6}>
                                    <Text>{value}</Text>
                                    {decodedNames.includes(value) && (
                                        <Tag color="green" style={{marginInlineEnd: 0}}>
                                            {t("VirtualFileTable.decoded")}
                                        </Tag>
                                    )}
                                </Space>
                            ),
                        },
                        {
                            title: t("VirtualFileTable.position"),
                            dataIndex: "position",
                            width: 140,
                            align: "right",
                            sorter: (left, right) => left.position - right.position,
                            render: (value: number) => <span className="mono">{formatNumber(value)}</span>,
                        },
                        {
                            title: t("VirtualFileTable.size"),
                            dataIndex: "size",
                            width: 120,
                            align: "right",
                            sorter: (left, right) => left.size - right.size,
                            render: (value: number) => <span className="mono">{formatBytes(value)}</span>,
                        },
                        {
                            title: "",
                            key: "actions",
                            width: 110,
                            render: (_, row) => (
                                <Button
                                    size="small"
                                    icon={<DownloadOutlined/>}
                                    loading={busyName === row.name}
                                    onClick={() => void extract(row)}
                                >
                                    {t("VirtualFileTable.extract")}
                                </Button>
                            ),
                        },
                    ]}
                />
            </div>
        </div>
    );
};

export default VirtualFileTable;
