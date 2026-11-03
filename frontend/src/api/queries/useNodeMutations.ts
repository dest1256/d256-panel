import { useMutation, useQueryClient } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { keys } from '@/api/queryKeys';

export interface NodeFormPayload {
  name: string;
  remark: string;
  address: string;
}

// Returned by create and enrollToken; the token is shown once and never stored in plaintext.
export interface NodeEnrollment {
  nodeId: number;
  token: string;
}

export function useNodeMutations() {
  const queryClient = useQueryClient();
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: keys.nodes.root() });
    queryClient.invalidateQueries({ queryKey: keys.inbounds.options() });
  };
  const onSuccess = (msg: { success?: boolean } | undefined) => {
    if (msg?.success) invalidate();
  };

  const createMut = useMutation({
    mutationFn: (payload: NodeFormPayload) =>
      HttpUtil.post<NodeEnrollment>('/panel/api/nodes/create', payload),
    onSuccess,
  });
  const updateMut = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: NodeFormPayload }) =>
      HttpUtil.post(`/panel/api/nodes/update/${id}`, payload),
    onSuccess,
  });
  const removeMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/nodes/del/${id}`),
    onSuccess,
  });
  const setEnableMut = useMutation({
    mutationFn: ({ id, enable }: { id: number; enable: boolean }) =>
      HttpUtil.post(`/panel/api/nodes/setEnable/${id}`, { enable }),
    onSuccess,
  });
  const enrollTokenMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post<NodeEnrollment>(`/panel/api/nodes/enrollToken/${id}`),
    onSuccess,
  });

  return {
    create: (payload: NodeFormPayload) => createMut.mutateAsync(payload),
    update: (id: number, payload: NodeFormPayload) => updateMut.mutateAsync({ id, payload }),
    remove: (id: number) => removeMut.mutateAsync(id),
    setEnable: (id: number, enable: boolean) => setEnableMut.mutateAsync({ id, enable }),
    newEnrollToken: (id: number) => enrollTokenMut.mutateAsync(id),
  };
}
