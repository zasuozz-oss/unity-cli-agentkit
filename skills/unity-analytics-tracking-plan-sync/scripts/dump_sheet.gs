/** @OnlyCurrentDoc */
// Dump a tracking-plan tab so the agent can map columns by header text and
// use real sheet row numbers. Read the output from the execution log.

const CONFIG = {
  sheetName: '<MAIN_TAB_NAME>', // or leave '' and set gid
  gid: null,                    // e.g. <MAIN_GID> as a number
  headerRow: 1,                 // row holding "Event", "Param", "Data Type"...
  maxRows: 2000,
};

function dumpSheet() {
  const ss = SpreadsheetApp.getActiveSpreadsheet();
  Logger.log('TABS ' + ss.getSheets().map(s => s.getName() + '#' + s.getSheetId()).join(' | '));

  const sh = CONFIG.sheetName
    ? ss.getSheetByName(CONFIG.sheetName)
    : ss.getSheets().find(s => s.getSheetId() === CONFIG.gid);
  if (!sh) throw new Error('Tab not found: ' + (CONFIG.sheetName || CONFIG.gid));

  const lastRow = Math.min(sh.getLastRow(), CONFIG.maxRows);
  const lastCol = sh.getLastColumn();
  const range = sh.getRange(1, 1, lastRow, lastCol);
  const values = range.getDisplayValues();
  const rich = range.getRichTextValues();

  const header = values[CONFIG.headerRow - 1];
  Logger.log('HEADER ' + header.map((h, i) => (i + 1) + '=' + h.trim()).join(' | '));

  for (let r = 0; r < lastRow; r++) {
    for (let c = 0; c < lastCol; c++) {
      const v = values[r][c];
      if (v !== '') Logger.log((r + 1) + '|' + (c + 1) + '|' + v.replace(/\n/g, '\\n'));
      const link = rich[r][c] && rich[r][c].getLinkUrl();
      if (link) Logger.log('LINK ' + (r + 1) + '|' + (c + 1) + '|' + link);
    }
  }

  range.getMergedRanges().forEach(m => Logger.log('MERGE ' + m.getA1Notation()));
}
