import { Alert, Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle } from '@mui/material';

export function UnsavedChangesDialog({ open, onKeepEditing, onDiscard }: { open: boolean; onKeepEditing: () => void; onDiscard: () => void }) {
  return (
    <Dialog open={open} onClose={onKeepEditing} maxWidth="xs" fullWidth>
      <DialogTitle>저장하지 않은 변경이 있습니다</DialogTitle>
      <DialogContent>
        <DialogContentText>지금 이동하면 편집한 내용이 사라집니다.</DialogContentText>
        <Alert severity="warning" sx={{ mt: 2 }}>Draft 저장 후 이동하면 내용이 보존됩니다.</Alert>
      </DialogContent>
      <DialogActions>
        <Button onClick={onKeepEditing} autoFocus>계속 편집</Button>
        <Button color="error" onClick={onDiscard}>변경 버리고 이동</Button>
      </DialogActions>
    </Dialog>
  );
}
