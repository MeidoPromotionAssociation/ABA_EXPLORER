import React, {useState} from "react";
import {Collapse, Descriptions, Space, Tag, Typography} from "antd";
import {useTranslation} from "react-i18next";
import type {CtOverview} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import {CtOverviewOpenKey} from "../../utils/LocalStorageKeys";
import {formatBytes, formatNumber} from "../../utils/format";

const {Text} = Typography;

/**
 * CtOverviewPanel 内容表头部信息面板
 * 默认折叠，把纵向空间让给 catalog 条目表；折叠时标题栏显示 catalog 名、分类与条目数
 */
const CtOverviewPanel: React.FC<{ overview: CtOverview }> = ({overview}) => {
    const {t} = useTranslation();
    const [open, setOpen] = useState(() => localStorage.getItem(CtOverviewOpenKey) === "true");

    const catalog = overview.catalog;

    const summary = (
        <Space size={8} wrap>
            <Text strong style={{fontSize: 13}}>{catalog?.name || t("CtPage.no_catalog")}</Text>
            {catalog && <Tag color="blue" style={{marginInlineEnd: 0}}>{catalog.kind}</Tag>}
            {catalog?.catalogTypeNames.map((name) => (
                <Tag key={name} color="geekblue" style={{marginInlineEnd: 0}}>{name}</Tag>
            ))}
            <Text type="secondary" style={{fontSize: 12}}>{formatBytes(overview.fileSize)}</Text>
            <Text type="secondary" style={{fontSize: 12}}>
                {t("CatalogItemTable.count", {
                    shown: formatNumber(catalog?.items.length ?? 0),
                    total: formatNumber(catalog?.items.length ?? 0),
                })}
            </Text>
        </Space>
    );

    return (
        <Collapse
            size="small"
            style={{marginBottom: 12, flex: "none"}}
            activeKey={open ? ["overview"] : []}
            onChange={(keys) => {
                const next = keys.length > 0;
                setOpen(next);
                localStorage.setItem(CtOverviewOpenKey, String(next));
            }}
            items={[{
                key: "overview",
                label: summary,
                children: (
                    <Descriptions
                        size="small"
                        column={{xs: 1, sm: 2, lg: 3, xxl: 4}}
                        items={[
                            {
                                key: "name",
                                label: t("CtPage.catalog_name"),
                                children: catalog?.name || "-",
                            },
                            {
                                key: "kind",
                                label: t("CtPage.kind"),
                                children: catalog ? <Tag color="blue">{catalog.kind}</Tag> : "-",
                            },
                            {
                                key: "catalogType",
                                label: t("CtPage.catalog_type"),
                                children: catalog ? (
                                    <Space size={4} wrap>
                                        {catalog.catalogTypeNames.map((name) => (
                                            <Tag key={name} color="geekblue" style={{marginInlineEnd: 0}}>{name}</Tag>
                                        ))}
                                        <Text type="secondary" className="mono">({catalog.catalogType})</Text>
                                    </Space>
                                ) : "-",
                                span: 2,
                            },
                            {
                                key: "packageType",
                                label: t("CtPage.package_type"),
                                children: catalog ? `${catalog.packageTypeName} (${catalog.packageType})` : "-",
                            },
                            {
                                key: "priority",
                                label: t("CtPage.priority"),
                                children: catalog ? formatNumber(catalog.priority) : "-",
                            },
                            {
                                key: "version",
                                label: t("CtPage.version"),
                                children: t("CtPage.version_value", {
                                    directory: overview.version,
                                    catalog: catalog?.version ?? "-",
                                }),
                            },
                            {
                                key: "framing",
                                label: t("CtPage.framing"),
                                children: overview.framing,
                            },
                            {
                                key: "fileSize",
                                label: t("CtPage.file_size"),
                                children: formatBytes(overview.fileSize),
                            },
                            {
                                key: "encrypted",
                                label: t("CtPage.encrypted"),
                                children: catalog?.isEncrypted ? t("Common.yes") : t("Common.no"),
                            },
                            {
                                key: "hash",
                                label: t("CtPage.hash"),
                                children: catalog ? <Text className="mono" copyable>{catalog.hash}</Text> : "-",
                            },
                            {
                                key: "createTime",
                                label: t("CtPage.create_time"),
                                children: <Text className="mono">{catalog?.createTime ?? "-"}</Text>,
                            },
                            {
                                key: "resourceFileNames",
                                label: t("CtPage.resource_files"),
                                children: catalog && catalog.resourceFileNames.length > 0
                                    ? catalog.resourceFileNames.join(", ")
                                    : "-",
                                span: 2,
                            },
                        ]}
                    />
                ),
            }]}
        />
    );
};

export default CtOverviewPanel;
