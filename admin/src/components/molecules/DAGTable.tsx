import React from 'react';
import {
  createTable,
  useTableInstance,
  getCoreRowModel,
  getSortedRowModel,
  SortingState,
  getFilteredRowModel,
  ColumnFiltersState,
  ExpandedState,
  getExpandedRowModel,
} from '@tanstack/react-table';
import DAGActions from './DAGActions';
import StatusChip from '../atoms/StatusChip';
import {
  Autocomplete,
  Box,
  Chip,
  IconButton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
} from '@mui/material';
import { Link } from 'react-router-dom';
import {
  getFirstTag,
  getStatus,
  getStatusField,
  DAGItem,
  DAGDataType,
  getNextSchedule,
} from '../../models';
import StyledTableRow from '../atoms/StyledTableRow';
import {
  ArrowDownward,
  ArrowUpward,
  KeyboardArrowDown,
  KeyboardArrowUp,
} from '@mui/icons-material';
import LiveSwitch from './LiveSwitch';
import moment from 'moment';
import 'moment-duration-format';
import Ticker from '../atoms/Ticker';

type Props = {
  DAGs: DAGItem[];
  group: string;
  refreshFn: () => void;
};

type DAGRow = DAGItem & { subRows?: DAGItem[] };

const durFormatSec = 's[s]m[m]h[h]d[d]';
const durFormatMin = 'm[m]h[h]d[d]';

const table = createTable()
  .setRowType<DAGRow>()
  .setFilterMetaType<DAGRow>()
  .setTableMetaType<{
    group: string;
    refreshFn: () => void;
  }>();

const defaultColumns = [
  table.createDataColumn('Name', {
    id: 'Expand',
    header: ({ instance }) => {
      return (
        <IconButton
          onClick={instance.getToggleAllRowsExpandedHandler()}
          sx={{
            color: 'white',
          }}
        >
          {instance.getIsAllRowsExpanded() ? (
            <KeyboardArrowUp />
          ) : (
            <KeyboardArrowDown />
          )}
        </IconButton>
      );
    },
    cell: ({ row }) => {
      if (row.getCanExpand()) {
        return (
          <IconButton onClick={row.getToggleExpandedHandler()}>
            {row.getIsExpanded() ? <KeyboardArrowUp /> : <KeyboardArrowDown />}
          </IconButton>
        );
      }
      return '';
    },
    enableSorting: false,
  }),
  table.createDataColumn('Name', {
    id: 'Name',
    cell: ({ row, getValue }) => {
      const data = row.original!;
      if (data.Type == DAGDataType.Group) {
        return getValue();
      } else {
        const name = data.DAGStatus.File.replace(/.y[a]{0,1}ml$/, '');
        const url = `/dags/${encodeURI(name)}`;
        return (
          <div
            style={{
              paddingLeft: `${row.depth * 2}rem`,
            }}
          >
            <Link to={url}>{getValue()}</Link>
          </div>
        );
      }
    },
    filterFn: (props, _, filter) => {
      const data = props.original!;
      let value = '';
      if (data.Type == DAGDataType.Group) {
        value = data.Name;
      } else {
        value = data.DAGStatus.DAG.Name;
      }
      const ret = value.toLowerCase().includes(filter.toLowerCase());
      return ret;
    },
    sortingFn: (a, b) => {
      const dataA = a.original!.Name.toLowerCase();
      const dataB = b.original!.Name.toLowerCase();
      return dataA.localeCompare(dataB);
    },
  }),
  table.createDataColumn('Type', {
    id: 'Tags',
    header: 'Tags',
    cell: (props) => {
      const data = props.row.original!;
      if (data.Type == DAGDataType.DAG) {
        const tags = data.DAGStatus.DAG.Tags;
        return (
          <Stack direction="row" spacing={1}>
            {tags?.map((tag) => (
              <Chip
                key={tag}
                size="small"
                label={tag}
                onClick={() => props.column.setFilterValue(tag)}
              />
            ))}
          </Stack>
        );
      }
      return null;
    },
    filterFn: (props, _, filter) => {
      const data = props.original!;
      if (data.Type != DAGDataType.DAG) {
        return false;
      }
      const tags = data.DAGStatus.DAG.Tags;
      const ret = tags?.some((tag) => tag == filter) || false;
      return ret;
    },
    sortingFn: (a, b) => {
      const valA = getFirstTag(a.original);
      const valB = getFirstTag(b.original);
      return valA.localeCompare(valB);
    },
  }),
  table.createDataColumn('Type', {
    id: 'Status',
    header: 'Status',
    cell: (props) => {
      const data = props.row.original!;
      if (data.Type == DAGDataType.DAG) {
        return (
          <StatusChip status={data.DAGStatus.Status?.Status}>
            {data.DAGStatus.Status?.StatusText || ''}
          </StatusChip>
        );
      }
      return null;
    },
    sortingFn: (a, b) => {
      const valA = getStatus(a.original);
      const valB = getStatus(b.original);
      return valA < valB ? -1 : 1;
    },
  }),
  table.createDataColumn('Type', {
    id: 'Started At',
    header: 'Started At',
    cell: (props) => {
      const data = props.row.original!;
      if (data.Type == DAGDataType.DAG) {
        return data.DAGStatus.Status?.StartedAt;
      }
      return null;
    },
    sortingFn: (a, b) => {
      const dataA = a.original!;
      const dataB = b.original!;
      const valA = getStatusField('StartedAt', dataA);
      const valB = getStatusField('StartedAt', dataB);
      return valA.localeCompare(valB);
    },
  }),
  table.createDataColumn('Type', {
    id: 'Finished At',
    header: 'Finished At',
    cell: (props) => {
      const data = props.row.original!;
      if (data.Type == DAGDataType.DAG) {
        return data.DAGStatus.Status?.FinishedAt;
      }
      return null;
    },
    sortingFn: (a, b) => {
      const dataA = a.original!;
      const dataB = b.original!;
      const valA = getStatusField('FinishedAt', dataA);
      const valB = getStatusField('FinishedAt', dataB);
      return valA.localeCompare(valB);
    },
  }),
  table.createDataColumn('Type', {
    id: 'Schedule',
    header: 'Schedule',
    enableSorting: true,
    cell: (props) => {
      const data = props.row.original!;
      if (data.Type == DAGDataType.DAG) {
        const schedules = data.DAGStatus.DAG.ScheduleExp;
        if (schedules) {
          return (
            <React.Fragment>
              {schedules.map((s) => (
                <Chip
                  key={s}
                  sx={{
                    fontWeight: 'semibold',
                    marginRight: 1,
                  }}
                  size="small"
                  label={s}
                />
              ))}
            </React.Fragment>
          );
        }
      }
      return null;
    },
    sortingFn: (a, b) => {
      const dataA = a.original!;
      const dataB = b.original!;
      if (dataA.Type != DAGDataType.DAG || dataB.Type != DAGDataType.DAG) {
        return dataA!.Type - dataB!.Type;
      }
      return (
        getNextSchedule(dataA.DAGStatus) - getNextSchedule(dataB.DAGStatus)
      );
    },
  }),
  table.createDataColumn('Type', {
    id: 'NextRun',
    header: 'Next Run',
    enableSorting: true,
    cell: (props) => {
      const data = props.row.original!;
      if (data.Type == DAGDataType.DAG) {
        const schedules = data.DAGStatus.DAG.ScheduleExp;
        if (schedules && !data.DAGStatus.Suspended) {
          return (
            <React.Fragment>
              in{' '}
              <Ticker intervalMs={1000}>
                {() => {
                  const ms = moment
                    .unix(getNextSchedule(data.DAGStatus))
                    .diff(moment.now());
                  const format = ms / 1000 > 60 ? durFormatMin : durFormatSec;
                  return (
                    <span>
                      {moment
                        .duration(ms)
                        // eslint-disable-next-line @typescript-eslint/ban-ts-comment
                        // @ts-ignore
                        .format(format)}
                    </span>
                  );
                }}
              </Ticker>
            </React.Fragment>
          );
        }
      }
      return null;
    },
    sortingFn: (a, b) => {
      const dataA = a.original!;
      const dataB = b.original!;
      if (dataA.Type != DAGDataType.DAG || dataB.Type != DAGDataType.DAG) {
        return dataA!.Type - dataB!.Type;
      }
      return (
        getNextSchedule(dataA.DAGStatus) - getNextSchedule(dataB.DAGStatus)
      );
    },
  }),
  table.createDataColumn('Type', {
    id: 'Config',
    header: 'Description',
    enableSorting: false,
    cell: (props) => {
      const data = props.row.original!;
      if (data.Type == DAGDataType.DAG) {
        return data.DAGStatus.DAG.Description;
      }
      return null;
    },
  }),
  table.createDataColumn('Type', {
    id: 'On/Off',
    header: 'Live',
    cell: (props) => {
      const data = props.row.original!;
      if (data.Type != DAGDataType.DAG) {
        return false;
      }
      return (
        <LiveSwitch
          DAG={data.DAGStatus}
          refresh={props.instance.options.meta?.refreshFn}
        />
      );
    },
  }),
  table.createDisplayColumn({
    id: 'Actions',
    header: 'Actions',
    cell: (props) => {
      const data = props.row.original!;
      if (data.Type == DAGDataType.Group) {
        return null;
      }
      return (
        <DAGActions
          status={data.DAGStatus.Status}
          name={data.DAGStatus.DAG.Name}
          label={false}
          refresh={props.instance.options.meta?.refreshFn}
        />
      );
    },
  }),
];

function DAGTable({ DAGs = [], group = '', refreshFn }: Props) {
  const [columns] = React.useState<typeof defaultColumns>(() => [
    ...defaultColumns,
  ]);

  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    []
  );
  const [sorting, setSorting] = React.useState<SortingState>([
    {
      id: 'Name',
      desc: false,
    },
  ]);

  const selectedTag = React.useMemo(() => {
    return (
      (columnFilters.find((filter) => filter.id == 'Tags')?.value as string) ||
      ''
    );
  }, [columnFilters]);

  const [expanded, setExpanded] = React.useState<ExpandedState>({});

  const data = React.useMemo(() => {
    const groups: {
      [key: string]: DAGRow;
    } = {};
    DAGs.forEach((dag) => {
      if (dag.Type == DAGDataType.DAG) {
        const g = dag.DAGStatus.DAG.Group;
        if (g != '') {
          if (!groups[g]) {
            groups[g] = {
              Type: DAGDataType.Group,
              Name: g,
              subRows: [],
            };
          }
          groups[g].subRows!.push(dag);
        }
      }
    });
    const ret: DAGRow[] = [];
    const groupKeys = Object.keys(groups);
    groupKeys.forEach((key) => {
      ret.push(groups[key]);
    });
    return [
      ...ret,
      ...DAGs.filter(
        (dag) =>
          dag.Type == DAGDataType.DAG &&
          dag.DAGStatus.DAG.Group == '' &&
          dag.DAGStatus.DAG.Group == group
      ),
    ];
  }, [DAGs, group]);

  const tagOptions = React.useMemo(() => {
    const map: { [key: string]: boolean } = { '': true };
    DAGs.forEach((data) => {
      if (data.Type == DAGDataType.DAG) {
        data.DAGStatus.DAG.Tags?.forEach((tag) => {
          map[tag] = true;
        });
      }
    });
    const ret = Object.keys(map).sort();
    return ret;
  }, []);

  const instance = useTableInstance(table, {
    data,
    columns,
    getSubRows: (row) => row.subRows,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnFiltersChange: setColumnFilters,
    getExpandedRowModel: getExpandedRowModel(),
    autoResetExpanded: false,
    state: {
      sorting,
      expanded,
      columnFilters,
    },
    onSortingChange: setSorting,
    onExpandedChange: setExpanded,
    debugAll: true,
    meta: {
      group,
      refreshFn,
    },
  });

  React.useEffect(() => {
    instance.toggleAllRowsExpanded(true);
  }, []);

  return (
    <Box>
      <Stack
        sx={{
          flexDirection: 'row',
          alignItems: 'center',
          justifyContent: 'start',
          alignContent: 'flex-center',
        }}
      >
        <TextField
          label="Search Text"
          size="small"
          variant="filled"
          InputProps={{
            value: instance.getColumn('Name').getFilterValue(),
            onChange: (e) => {
              instance.getColumn('Name').setFilterValue(e.target.value || '');
            },
            type: 'search',
          }}
        />
        <Autocomplete<string>
          size="small"
          limitTags={1}
          value={selectedTag}
          options={tagOptions}
          onChange={(_, value) => {
            instance.getColumn('Tags').setFilterValue(value || '');
          }}
          renderInput={(params) => (
            <TextField {...params} variant="filled" label="Search Tag" />
          )}
          sx={{ width: '300px', ml: 2 }}
        />
      </Stack>
      <Box
        sx={{
          border: '1px solid #6149d8',
          borderRadius: '6px',
          mt: 2,
        }}
      >
        <Table size="small">
          <TableHead>
            {instance.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableCell
                    key={header.id}
                    style={{
                      padding:
                        header.id == 'Expand' || header.id == 'Name'
                          ? '6px 4px'
                          : '6px 16px',
                    }}
                  >
                    {header.column.getCanSort() ? (
                      <Box
                        {...{
                          sx: {
                            cursor: header.column.getCanSort()
                              ? 'pointer'
                              : 'default',
                          },
                          onClick: header.column.getToggleSortingHandler(),
                        }}
                      >
                        <Stack direction="row" alignItems="center">
                          {header.isPlaceholder ? null : header.renderHeader()}
                          {{
                            asc: (
                              <ArrowUpward
                                sx={{
                                  color: 'white',
                                  fontSize: '0.95rem',
                                  ml: 1,
                                }}
                              />
                            ),
                            desc: (
                              <ArrowDownward
                                sx={{
                                  color: 'white',
                                  fontSize: '0.95rem',
                                  ml: 1,
                                }}
                              />
                            ),
                          }[header.column.getIsSorted() as string] ?? null}
                        </Stack>
                      </Box>
                    ) : (
                      header.renderHeader()
                    )}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableHead>
          <TableBody>
            {instance.getRowModel().rows.map((row) => (
              <StyledTableRow key={row.id} style={{ height: '44px' }}>
                {row.getVisibleCells().map((cell) => (
                  <TableCell
                    key={cell.id}
                    style={{
                      padding:
                        cell.column.id == 'Expand' || cell.column.id == 'Name'
                          ? '6px 4px'
                          : '6px 16px',
                    }}
                    width={cell.column.id == 'Expand' ? '44px' : undefined}
                  >
                    {cell.renderCell()}
                  </TableCell>
                ))}
              </StyledTableRow>
            ))}
          </TableBody>
        </Table>
      </Box>
    </Box>
  );
}
export default DAGTable;
