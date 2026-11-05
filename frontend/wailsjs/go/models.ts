export namespace api {
	
	export class GroupSuggestion {
	    groupId: number;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new GroupSuggestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groupId = source["groupId"];
	        this.name = source["name"];
	    }
	}
	export class Library {
	
	
	    static createFrom(source: any = {}) {
	        return new Library(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class TrashSnapshot {
	    videos: video.Video[];
	    retentionDays: number;
	
	    static createFrom(source: any = {}) {
	        return new TrashSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.videos = this.convertValues(source["videos"], video.Video);
	        this.retentionDays = source["retentionDays"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VideoGroupSuggestion {
	    path: string;
	    fileName: string;
	    suggested: GroupSuggestion[];
	
	    static createFrom(source: any = {}) {
	        return new VideoGroupSuggestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.fileName = source["fileName"];
	        this.suggested = this.convertValues(source["suggested"], GroupSuggestion);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace cloud {
	
	export class CacheInfo {
	    files: number;
	    bytes: number;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new CacheInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.bytes = source["bytes"];
	        this.limit = source["limit"];
	    }
	}
	export class ConnectionInput {
	    id: number;
	    name: string;
	    kind: string;
	    endpoint: string;
	    webUrl: string;
	    username: string;
	    password: string;
	    clearPassword: boolean;
	    rootPath: string;
	    verifyTls: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.endpoint = source["endpoint"];
	        this.webUrl = source["webUrl"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.clearPassword = source["clearPassword"];
	        this.rootPath = source["rootPath"];
	        this.verifyTls = source["verifyTls"];
	    }
	}
	export class ConnectionView {
	    id: number;
	    name: string;
	    kind: string;
	    endpoint: string;
	    webUrl: string;
	    username: string;
	    rootPath: string;
	    verifyTls: boolean;
	    hasPassword: boolean;
	    online: boolean;
	    lastError: string;
	    lastOkAt: number;
	    videoCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.endpoint = source["endpoint"];
	        this.webUrl = source["webUrl"];
	        this.username = source["username"];
	        this.rootPath = source["rootPath"];
	        this.verifyTls = source["verifyTls"];
	        this.hasPassword = source["hasPassword"];
	        this.online = source["online"];
	        this.lastError = source["lastError"];
	        this.lastOkAt = source["lastOkAt"];
	        this.videoCount = source["videoCount"];
	    }
	}
	export class Entry {
	    name: string;
	    path: string;
	    isDir: boolean;
	    size: number;
	    modified: number;
	    contentType: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.modified = source["modified"];
	        this.contentType = source["contentType"];
	    }
	}
	export class TestResult {
	    ok: boolean;
	    latencyMs: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.latencyMs = source["latencyMs"];
	        this.error = source["error"];
	    }
	}

}

export namespace group {
	
	export class Group {
	    id: number;
	    name: string;
	    position: number;
	    parentId?: number;
	    videoCount: number;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.position = source["position"];
	        this.parentId = source["parentId"];
	        this.videoCount = source["videoCount"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

export namespace jav {
	
	export class ActorFillStatus {
	    total: number;
	    queued: number;
	    running: number;
	    succeeded: number;
	    notFound: number;
	    failed: number;
	    current?: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ActorFillStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.queued = source["queued"];
	        this.running = source["running"];
	        this.succeeded = source["succeeded"];
	        this.notFound = source["notFound"];
	        this.failed = source["failed"];
	        this.current = source["current"];
	        this.active = source["active"];
	    }
	}
	export class ActorProfile {
	    name: string;
	    provider?: string;
	    starId?: string;
	    avatarUrl?: string;
	    avatarPath?: string;
	    birthday?: string;
	    heightCm?: number;
	    bustCm?: number;
	    cup?: string;
	    waistCm?: number;
	    hipCm?: number;
	    bloodType?: string;
	    hometown?: string;
	    hobby?: string;
	    movieCount?: number;
	    lastReleaseUnix?: number;
	    aliases?: string[];
	    fetchedAt?: number;
	
	    static createFrom(source: any = {}) {
	        return new ActorProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.starId = source["starId"];
	        this.avatarUrl = source["avatarUrl"];
	        this.avatarPath = source["avatarPath"];
	        this.birthday = source["birthday"];
	        this.heightCm = source["heightCm"];
	        this.bustCm = source["bustCm"];
	        this.cup = source["cup"];
	        this.waistCm = source["waistCm"];
	        this.hipCm = source["hipCm"];
	        this.bloodType = source["bloodType"];
	        this.hometown = source["hometown"];
	        this.hobby = source["hobby"];
	        this.movieCount = source["movieCount"];
	        this.lastReleaseUnix = source["lastReleaseUnix"];
	        this.aliases = source["aliases"];
	        this.fetchedAt = source["fetchedAt"];
	    }
	}
	export class ActorSummary {
	    name: string;
	    count: number;
	    aliases?: string[];
	    profile?: ActorProfile;
	
	    static createFrom(source: any = {}) {
	        return new ActorSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.count = source["count"];
	        this.aliases = source["aliases"];
	        this.profile = this.convertValues(source["profile"], ActorProfile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Detail {
	    videoId: number;
	    code: string;
	    title: string;
	    studio: string;
	    series: string;
	    releaseUnix?: number;
	    durationMin?: number;
	    isUncensored?: boolean;
	    actors: string[];
	    tags: string[];
	    coverUrl: string;
	    coverStatus: string;
	    provider: string;
	    codeSource: string;
	    overrideMode: string;
	    scrapeStatus: string;
	    attempts: number;
	    lastError: string;
	    nextRetryAt?: number;
	    fetchedAt?: number;
	    errorCode: string;
	    errorAt?: number;
	
	    static createFrom(source: any = {}) {
	        return new Detail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.videoId = source["videoId"];
	        this.code = source["code"];
	        this.title = source["title"];
	        this.studio = source["studio"];
	        this.series = source["series"];
	        this.releaseUnix = source["releaseUnix"];
	        this.durationMin = source["durationMin"];
	        this.isUncensored = source["isUncensored"];
	        this.actors = source["actors"];
	        this.tags = source["tags"];
	        this.coverUrl = source["coverUrl"];
	        this.coverStatus = source["coverStatus"];
	        this.provider = source["provider"];
	        this.codeSource = source["codeSource"];
	        this.overrideMode = source["overrideMode"];
	        this.scrapeStatus = source["scrapeStatus"];
	        this.attempts = source["attempts"];
	        this.lastError = source["lastError"];
	        this.nextRetryAt = source["nextRetryAt"];
	        this.fetchedAt = source["fetchedAt"];
	        this.errorCode = source["errorCode"];
	        this.errorAt = source["errorAt"];
	    }
	}
	export class DuplicateGroup {
	    key: string;
	    code: string;
	    part: string;
	    ignored: boolean;
	    videos: video.Video[];
	
	    static createFrom(source: any = {}) {
	        return new DuplicateGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.code = source["code"];
	        this.part = source["part"];
	        this.ignored = source["ignored"];
	        this.videos = this.convertValues(source["videos"], video.Video);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Task {
	    id: number;
	    videoId: number;
	    status: string;
	    force: boolean;
	    attempts: number;
	    maxAttempts: number;
	    nextRetryAt?: number;
	    leaseUntil?: number;
	    errorCode: string;
	    lastError: string;
	    errorAt?: number;
	    createdAt: number;
	    updatedAt: number;
	    startedAt?: number;
	    finishedAt?: number;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.videoId = source["videoId"];
	        this.status = source["status"];
	        this.force = source["force"];
	        this.attempts = source["attempts"];
	        this.maxAttempts = source["maxAttempts"];
	        this.nextRetryAt = source["nextRetryAt"];
	        this.leaseUntil = source["leaseUntil"];
	        this.errorCode = source["errorCode"];
	        this.lastError = source["lastError"];
	        this.errorAt = source["errorAt"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	    }
	}
	export class TaskOverview {
	    id: number;
	    videoId: number;
	    status: string;
	    force: boolean;
	    attempts: number;
	    maxAttempts: number;
	    nextRetryAt?: number;
	    leaseUntil?: number;
	    errorCode: string;
	    lastError: string;
	    errorAt?: number;
	    createdAt: number;
	    updatedAt: number;
	    startedAt?: number;
	    finishedAt?: number;
	    category: string;
	    video: video.Video;
	
	    static createFrom(source: any = {}) {
	        return new TaskOverview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.videoId = source["videoId"];
	        this.status = source["status"];
	        this.force = source["force"];
	        this.attempts = source["attempts"];
	        this.maxAttempts = source["maxAttempts"];
	        this.nextRetryAt = source["nextRetryAt"];
	        this.leaseUntil = source["leaseUntil"];
	        this.errorCode = source["errorCode"];
	        this.lastError = source["lastError"];
	        this.errorAt = source["errorAt"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.category = source["category"];
	        this.video = this.convertValues(source["video"], video.Video);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class FC2CookieStatus {
	    configured: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FC2CookieStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	    }
	}
	export class JavBusCookieStatus {
	    configured: boolean;
	
	    static createFrom(source: any = {}) {
	        return new JavBusCookieStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	    }
	}

}

export namespace server {
	
	export class Server {
	
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class StartInfo {
	    running: boolean;
	    urls: string[];
	    local: string;
	    port: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new StartInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.urls = source["urls"];
	        this.local = source["local"];
	        this.port = source["port"];
	        this.error = source["error"];
	    }
	}

}

export namespace settings {
	
	export class Store {
	
	
	    static createFrom(source: any = {}) {
	        return new Store(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace video {
	
	export class AddVideoResult {
	    path: string;
	    success: boolean;
	    videoId?: number;
	    error?: string;
	    relocated?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AddVideoResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.success = source["success"];
	        this.videoId = source["videoId"];
	        this.error = source["error"];
	        this.relocated = source["relocated"];
	    }
	}
	export class Video {
	    id: number;
	    path: string;
	    fileName: string;
	    fileSize: number;
	    durationMs: number;
	    width: number;
	    height: number;
	    frameRate: number;
	    bitrate: number;
	    videoCodec: string;
	    audioCodec: string;
	    thumbnailPath: string;
	    createdAt: number;
	    updatedAt: number;
	    lastOpenedAt: number;
	    status: string;
	    volumeOffline: boolean;
	    volumeRoot: string;
	    title: string;
	    notes: string;
	    javCode: string;
	    mosaicRestored: boolean;
	    customCoverAt: number;
	    likeCount: number;
	    coverSource: string;
	    scrapedCoverPath: string;
	    allowScrapedCover: boolean;
	    contentHash: string;
	    hashVersion: number;
	    sourceId: number;
	    remotePath: string;
	    sourceName: string;
	    sourceOffline: boolean;
	    deletedAt: number;
	    deleteReason: string;
	    trashGroupNames: string[];
	
	    static createFrom(source: any = {}) {
	        return new Video(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.fileName = source["fileName"];
	        this.fileSize = source["fileSize"];
	        this.durationMs = source["durationMs"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.frameRate = source["frameRate"];
	        this.bitrate = source["bitrate"];
	        this.videoCodec = source["videoCodec"];
	        this.audioCodec = source["audioCodec"];
	        this.thumbnailPath = source["thumbnailPath"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.lastOpenedAt = source["lastOpenedAt"];
	        this.status = source["status"];
	        this.volumeOffline = source["volumeOffline"];
	        this.volumeRoot = source["volumeRoot"];
	        this.title = source["title"];
	        this.notes = source["notes"];
	        this.javCode = source["javCode"];
	        this.mosaicRestored = source["mosaicRestored"];
	        this.customCoverAt = source["customCoverAt"];
	        this.likeCount = source["likeCount"];
	        this.coverSource = source["coverSource"];
	        this.scrapedCoverPath = source["scrapedCoverPath"];
	        this.allowScrapedCover = source["allowScrapedCover"];
	        this.contentHash = source["contentHash"];
	        this.hashVersion = source["hashVersion"];
	        this.sourceId = source["sourceId"];
	        this.remotePath = source["remotePath"];
	        this.sourceName = source["sourceName"];
	        this.sourceOffline = source["sourceOffline"];
	        this.deletedAt = source["deletedAt"];
	        this.deleteReason = source["deleteReason"];
	        this.trashGroupNames = source["trashGroupNames"];
	    }
	}
	export class ContentDuplicateMember {
	    video: Video;
	    missing: boolean;
	    volumeOffline: boolean;
	    needsReverify: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ContentDuplicateMember(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.video = this.convertValues(source["video"], Video);
	        this.missing = source["missing"];
	        this.volumeOffline = source["volumeOffline"];
	        this.needsReverify = source["needsReverify"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ContentDuplicateGroup {
	    hash: string;
	    members: ContentDuplicateMember[];
	    totalBytes: number;
	    reclaimableBytes: number;
	    ignored: boolean;
	    confirmed: boolean;
	    refuted: boolean;
	    confirming: boolean;
	    needsReverify: boolean;
	    missing: number;
	
	    static createFrom(source: any = {}) {
	        return new ContentDuplicateGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.members = this.convertValues(source["members"], ContentDuplicateMember);
	        this.totalBytes = source["totalBytes"];
	        this.reclaimableBytes = source["reclaimableBytes"];
	        this.ignored = source["ignored"];
	        this.confirmed = source["confirmed"];
	        this.refuted = source["refuted"];
	        this.confirming = source["confirming"];
	        this.needsReverify = source["needsReverify"];
	        this.missing = source["missing"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ContentScanStatus {
	    running: boolean;
	    phase: string;
	    examined: number;
	    examineTotal: number;
	    total: number;
	    done: number;
	    hashed: number;
	    skipped: number;
	    failed: number;
	    startedAt: number;
	    finishedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new ContentScanStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.phase = source["phase"];
	        this.examined = source["examined"];
	        this.examineTotal = source["examineTotal"];
	        this.total = source["total"];
	        this.done = source["done"];
	        this.hashed = source["hashed"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	    }
	}
	export class FileScanStatus {
	    running: boolean;
	    total: number;
	    checked: number;
	    removed: number;
	    skipped: number;
	    revived: number;
	    offline: number;
	    remote: number;
	    error: string;
	    startedAt: number;
	    finishedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new FileScanStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.total = source["total"];
	        this.checked = source["checked"];
	        this.removed = source["removed"];
	        this.skipped = source["skipped"];
	        this.revived = source["revived"];
	        this.offline = source["offline"];
	        this.remote = source["remote"];
	        this.error = source["error"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	    }
	}
	export class FingerprintState {
	    Size: number;
	    Mtime: number;
	    At: number;
	    Error: string;
	    FullHash: string;
	    FullHashAt: number;
	
	    static createFrom(source: any = {}) {
	        return new FingerprintState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Size = source["Size"];
	        this.Mtime = source["Mtime"];
	        this.At = source["At"];
	        this.Error = source["Error"];
	        this.FullHash = source["FullHash"];
	        this.FullHashAt = source["FullHashAt"];
	    }
	}
	export class Service {
	
	
	    static createFrom(source: any = {}) {
	        return new Service(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	
	export class VolumeChange {
	    root: string;
	    online: boolean;
	    changed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VolumeChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.online = source["online"];
	        this.changed = source["changed"];
	    }
	}

}

export namespace watch {
	
	export class WatchedDir {
	    id: number;
	    path: string;
	    createdAt: number;
	    lastScanAt: number;
	
	    static createFrom(source: any = {}) {
	        return new WatchedDir(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.createdAt = source["createdAt"];
	        this.lastScanAt = source["lastScanAt"];
	    }
	}

}

