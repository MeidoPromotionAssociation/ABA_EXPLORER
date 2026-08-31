import React from "react";
import {Alert, Collapse, Descriptions, Empty, Table, Tag, Typography} from "antd";
import {useTranslation} from "react-i18next";
import type {AbaSerializedFile} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import {formatBytes, formatNumber} from "../../utils/format";

const {Text} = Typography;

/**
 * SerializedFileList 每个 SerializedFile 的头部与元数据
 * 外部引用条数很少，用 antd Table 就够，不需要虚拟滚动
 */
const SerializedFileList: React.FC<{ files: AbaSerializedFile[] }> = ({files}) => {
    const {t} = useTranslation();

    if (files.length === 0) {
        return <Empty description={t("SerializedFileList.empty")} image={Empty.PRESENTED_IMAGE_SIMPLE}/>;
    }

    return (
        <Collapse
            defaultActiveKey={files.length > 0 ? [String(files[0].directoryIndex)] : []}
            items={files.map((file) => ({
                key: String(file.directoryIndex),
                label: (
                    <span>
                        <Text strong>{file.name}</Text>
                        <Tag style={{marginInlineStart: 8}}>{file.unityVersion}</Tag>
                        <Text type="secondary" style={{fontSize: 12}}>
                            {t("SerializedFileList.object_count", {count: (file.assets ?? []).length})}
                        </Text>
                    </span>
                ),
                children: (
                    <>
                        {file.containerError && (
                            <Alert
                                type="warning"
                                showIcon
                                style={{marginBottom: 12}}
                                title={t("SerializedFileList.container_error")}
                                description={file.containerError}
                            />
                        )}
                        <Descriptions size="small" column={{xs: 1, sm: 2, lg: 3}} bordered
                                      items={[
                                          {
                                              key: "formatVersion",
                                              label: t("SerializedFileList.format_version"),
                                              children: file.formatVersion,
                                          },
                                          {
                                              key: "unityVersion",
                                              label: t("SerializedFileList.unity_version"),
                                              children: file.unityVersion || "-",
                                          },
                                          {
                                              key: "targetPlatform",
                                              label: t("SerializedFileList.target_platform"),
                                              children: file.targetPlatform,
                                          },
                                          {
                                              key: "typeTree",
                                              label: t("SerializedFileList.type_tree"),
                                              children: file.typeTreeEnabled ? t("Common.yes") : t("Common.no"),
                                          },
                                          {
                                              key: "typeCount",
                                              label: t("SerializedFileList.type_count"),
                                              children: formatNumber(file.typeCount),
                                          },
                                          {
                                              key: "endianness",
                                              label: t("SerializedFileList.endianness"),
                                              children: file.bigEndian ? "Big-Endian" : "Little-Endian",
                                          },
                                          {
                                              key: "fileSize",
                                              label: t("SerializedFileList.file_size"),
                                              children: formatBytes(file.fileSize),
                                          },
                                          {
                                              key: "metadataSize",
                                              label: t("SerializedFileList.metadata_size"),
                                              children: formatBytes(file.metadataSize),
                                          },
                                          {
                                              key: "dataOffset",
                                              label: t("SerializedFileList.data_offset"),
                                              children: formatNumber(file.dataOffset),
                                          },
                                          ...(file.userInformation
                                              ? [{
                                                  key: "userInformation",
                                                  label: t("SerializedFileList.user_information"),
                                                  children: file.userInformation,
                                                  span: 3,
                                              }]
                                              : []),
                                      ]}
                        />

                        <Text strong style={{display: "block", marginTop: 16, marginBottom: 8}}>
                            {t("SerializedFileList.external_files")}
                        </Text>
                        <Table
                            size="small"
                            pagination={false}
                            rowKey={(row) => `${row.guid}:${row.pathName}`}
                            dataSource={file.externalFiles ?? []}
                            locale={{emptyText: t("SerializedFileList.no_external_files")}}
                            columns={[
                                {
                                    title: t("SerializedFileList.path_name"),
                                    dataIndex: "pathName",
                                    defaultSortOrder: "ascend",
                                    sorter: (left, right) =>
                                        left.pathName.localeCompare(right.pathName, undefined, {numeric: true}),
                                },
                                {
                                    title: t("SerializedFileList.asset_path"),
                                    dataIndex: "assetPath",
                                    sorter: (left, right) => left.assetPath.localeCompare(right.assetPath),
                                },
                                {
                                    title: "GUID",
                                    dataIndex: "guid",
                                    sorter: (left, right) => left.guid.localeCompare(right.guid),
                                    render: (value: string) => <span className="mono">{value}</span>,
                                },
                                {
                                    title: t("SerializedFileList.ref_type"),
                                    dataIndex: "type",
                                    width: 90,
                                    sorter: (left, right) => left.type - right.type,
                                },
                            ]}
                        />
                    </>
                ),
            }))}
        />
    );
};

export default SerializedFileList;
