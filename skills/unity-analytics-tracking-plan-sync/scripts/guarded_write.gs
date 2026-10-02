/** @OnlyCurrentDoc */
// Apply writes only where the row still holds what the agent expects.
// Columns are resolved by header text, never by position. Any mismatch,
// unknown header, or non-top-left merged cell is SKIPPED and logged.

const CONFIG = {
  sheetName: '<MAIN_TAB_NAME>',
  headerRow: 1,
  dryRun: true, // run once with true, read the log, then set false
  writes: [
    // { row: 42, expect: { 'Param': '<param_name>' }, set: { '<VERIFY_HEADER>': 'V' } },
    // { row: 43, expect: { 'Param': '<param_name>', 'Data Type': 'Number' },
    //   set: { 'Notes': '<product-level caveat>' } },
  ],
};

function guardedWrite() {
  const sh = SpreadsheetApp.getActiveSpreadsheet().getSheetByName(CONFIG.sheetName);
  if (!sh) throw new Error('Tab not found: ' + CONFIG.sheetName);

  const header = sh.getRange(CONFIG.headerRow, 1, 1, sh.getLastColumn()).getDisplayValues()[0];
  const col = name => {
    const i = header.findIndex(h => h.trim().toLowerCase() === name.trim().toLowerCase());
    return i < 0 ? null : i + 1;
  };

  let ok = 0, skipped = 0;
  CONFIG.writes.forEach(w => {
    const tag = 'row ' + w.row;
    const names = Object.keys(w.expect).concat(Object.keys(w.set));
    const missing = names.filter(n => col(n) === null);
    if (missing.length) { Logger.log('SKIP ' + tag + ' unknown header: ' + missing.join(', ')); skipped++; return; }

    const bad = Object.keys(w.expect).filter(n => {
      const cell = sh.getRange(w.row, col(n));
      const v = (cell.isPartOfMerge() ? cell.getMergedRanges()[0].getCell(1, 1) : cell).getDisplayValue();
      return v.trim() !== String(w.expect[n]).trim();
    });
    if (bad.length) {
      Logger.log('SKIP ' + tag + ' expect mismatch: ' + bad.map(n =>
        n + '="' + sh.getRange(w.row, col(n)).getDisplayValue() + '"').join(', '));
      skipped++; return;
    }

    Object.keys(w.set).forEach(n => {
      const cell = sh.getRange(w.row, col(n));
      if (cell.isPartOfMerge()) {
        const top = cell.getMergedRanges()[0];
        if (top.getRow() !== w.row || top.getColumn() !== col(n)) {
          Logger.log('SKIP ' + tag + ' ' + n + ' is inside merge ' + top.getA1Notation()); skipped++; return;
        }
      }
      Logger.log((CONFIG.dryRun ? 'DRY ' : 'SET ') + tag + ' ' + n + ': "' +
        cell.getDisplayValue() + '" -> "' + w.set[n] + '"');
      if (!CONFIG.dryRun) cell.setValue(w.set[n]);
      ok++;
    });
  });
  Logger.log('DONE ok=' + ok + ' skipped=' + skipped + (CONFIG.dryRun ? ' (dry run)' : ''));
}
