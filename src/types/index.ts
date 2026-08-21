import { Users } from '@internxt/sdk/dist/drive';

type RefreshUserResponse = Awaited<ReturnType<Users['refreshUser']>>;
export type UserData = RefreshUserResponse['user'];
